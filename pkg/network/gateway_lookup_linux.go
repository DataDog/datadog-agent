// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || linux_bpf

package network

import (
	"context"
	"math"
	"net"
	"os"
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/vishvananda/netns"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRouteCacheSize       = uint32(math.MaxUint32)
	maxSubnetCacheSize      = 1024
	gatewayLookupModuleName = "network__gateway_lookup"
)

// Telemetry
var gatewayLookupTelemetry = struct {
	subnetCacheSize    *telemetry.StatGaugeWrapper
	subnetCacheMisses  *telemetry.StatCounterWrapper
	subnetCacheLookups *telemetry.StatCounterWrapper
	subnetLookups      *telemetry.StatCounterWrapper
	subnetLookupErrors *telemetry.StatCounterWrapper
}{
	telemetry.NewStatGaugeWrapper(gatewayLookupModuleName, "subnet_cache_size", []string{}, "Counter measuring the size of the subnet cache"),
	telemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_cache_misses", []string{}, "Counter measuring the number of subnet cache misses"),
	telemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_cache_lookups", []string{}, "Counter measuring the number of subnet cache lookups"),
	telemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_lookups", []string{}, "Counter measuring the number of subnet lookups"),
	telemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_lookup_errors", []string{"reason"}, "Counter measuring the number of subnet lookup errors"),
}

type LinuxGatewayLookup struct {
	rootNetNs           netns.NsHandle
	lookupCaches        *lookupCaches
	subnetForHwAddrFunc func(net.HardwareAddr) (Subnet, error)
}

type lookupCaches struct {
	routeCache  RouteCache
	subnetCache *simplelru.LRU[int, interface{}] // interface index to subnet cache
}

type cloudProvider interface {
	IsAWS() bool
}

var cloud cloudProvider
var caches *lookupCaches
var once sync.Once

func init() {
	cloud = &cloudProviderImpl{}
}

func gwLookupEnabled() bool {
	// only enabled on AWS currently
	return cloud.IsAWS() && ddconfig.IsCloudProviderEnabled(ec2.CloudProviderName)
}

// NewGatewayLookup creates a new instance of a gateway lookup using
// a given root network namespace
func NewGatewayLookup() *LinuxGatewayLookup {
	if !gwLookupEnabled() {
		return nil
	}

	rootNetNS, err := netns.GetFromPid(os.Getpid())
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	lookupCaches, err := createCaches()
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}
	if lookupCaches == nil {
		log.Error("could not create gateway lookup: failed to create caches")
		return nil
	}

	return &LinuxGatewayLookup{
		rootNetNs:           rootNetNS,
		lookupCaches:        lookupCaches,
		subnetForHwAddrFunc: ec2SubnetForHardwareAddr,
	}
}

// NewTracerGatewayLookup creates a new gateway lookup for
// Network Tracer and takes in the tracer config
func NewTracerGatewayLookup(config *config.Config) *LinuxGatewayLookup {
	if !config.EnableGatewayLookup || !gwLookupEnabled() {
		return nil
	}

	ns, err := config.GetRootNetNs()
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	lookupCaches, err := createCaches()
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}
	if lookupCaches == nil {
		log.Error("could not create gateway lookup: failed to create caches")
		return nil
	}

	gl := &LinuxGatewayLookup{
		rootNetNs:           ns,
		lookupCaches:        lookupCaches,
		subnetForHwAddrFunc: ec2SubnetForHardwareAddr,
	}

	// TODO: do we want to ignore this since traceroute can have
	// an unknown number of routes in addition to MaxTrackedConnections?
	// this seems like it was just an optimization to same some memory
	// not sure how critical this piece is
	//
	// In theory, we could update the max size, but I'd prefer not to
	// routeCacheSize := maxRouteCacheSize
	// if config.MaxTrackedConnections <= maxRouteCacheSize {
	// 	routeCacheSize = config.MaxTrackedConnections
	// } else {
	// 	log.Warnf("using truncated route cache size of %d instead of %d", routeCacheSize, config.MaxTrackedConnections)
	// }

	return gl
}

func (g *LinuxGatewayLookup) Lookup(cs *ConnectionStats) *Via {
	dest := cs.Dest
	if cs.IPTranslation != nil {
		dest = cs.IPTranslation.ReplSrcIP
	}

	return g.LookupWithIPs(cs.Source, dest, cs.NetNS)
}

func (g *LinuxGatewayLookup) LookupWithIPs(source util.Address, dest util.Address, netns uint32) *Via {
	r, ok := g.lookupCaches.routeCache.Get(source, dest, netns)
	if !ok {
		return nil
	}

	// if there is no gateway, we don't need to add subnet info
	// for gateway resolution in the backend
	if r.Gateway.IsZero() || r.Gateway.IsUnspecified() {
		return nil
	}

	gatewayLookupTelemetry.subnetCacheLookups.Inc()
	v, ok := g.lookupCaches.subnetCache.Get(r.IfIndex)
	if !ok {
		gatewayLookupTelemetry.subnetCacheMisses.Inc()

		var s Subnet
		var err error
		err = kernel.WithNS(g.rootNetNs, func() error {
			var ifi *net.Interface
			ifi, err = net.InterfaceByIndex(r.IfIndex)
			if err != nil {
				log.Debugf("error getting interface for interface index %d: %s", r.IfIndex, err)
				// negative cache for 1 minute
				g.lookupCaches.subnetCache.Add(r.IfIndex, time.Now().Add(1*time.Minute))
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			if ifi.Flags&net.FlagLoopback != 0 {
				// negative cache loopback interfaces
				g.lookupCaches.subnetCache.Add(r.IfIndex, nil)
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			gatewayLookupTelemetry.subnetLookups.Inc()
			if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
				log.Debugf("error getting subnet info for interface index %d: %s", r.IfIndex, err)

				// cache an empty result so that we don't keep hitting the
				// ec2 metadata endpoint for this interface
				if errors.IsTimeout(err) {
					// retry after a minute if we timed out
					g.lookupCaches.subnetCache.Add(r.IfIndex, time.Now().Add(time.Minute))
					gatewayLookupTelemetry.subnetLookupErrors.Inc("timeout")
				} else {
					g.lookupCaches.subnetCache.Add(r.IfIndex, nil)
					gatewayLookupTelemetry.subnetLookupErrors.Inc("general error")
				}
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			return nil
		})

		if err != nil {
			return nil
		}

		via := &Via{Subnet: s}
		g.lookupCaches.subnetCache.Add(r.IfIndex, via)
		gatewayLookupTelemetry.subnetCacheSize.Inc()
		v = via
	} else if v == nil {
		return nil
	}

	switch cv := v.(type) {
	case time.Time:
		if time.Now().After(cv) {
			g.lookupCaches.subnetCache.Remove(r.IfIndex)
			gatewayLookupTelemetry.subnetCacheSize.Dec()
		}
		return nil
	case *Via:
		return cv
	default:
		return nil
	}
}

func (g *LinuxGatewayLookup) Close() {
	g.rootNetNs.Close()
	g.lookupCaches.Close()
}

func ec2SubnetForHardwareAddr(hwAddr net.HardwareAddr) (Subnet, error) {
	snet, err := ec2.GetSubnetForHardwareAddr(context.TODO(), hwAddr)
	if err != nil {
		return Subnet{}, err
	}

	return Subnet{Alias: snet.ID}, nil
}

type cloudProviderImpl struct{}

func (cp *cloudProviderImpl) IsAWS() bool {
	return ec2.IsRunningOn(context.TODO())
}

func (l *lookupCaches) Close() {
	l.routeCache.Close()
	l.purge()
}

func (l *lookupCaches) purge() {
	l.subnetCache.Purge()
	gatewayLookupTelemetry.subnetCacheSize.Set(0)
}

func createCaches() (*lookupCaches, error) {
	var err error
	once.Do(func() {
		var rootNetNS netns.NsHandle
		rootNetNS, err = netns.GetFromPid(os.Getpid())
		if err != nil {
			log.Errorf("could not create gateway lookup: %s", err)
			return
		}
		defer rootNetNS.Close() // TODO: do we want this here?

		var router Router
		router, err = NewNetlinkRouter(rootNetNS)
		if err != nil {
			log.Errorf("could not create gateway lookup: %s", err)
			return
		}

		var subnetCache *simplelru.LRU[int, interface{}]
		subnetCache, err = simplelru.NewLRU[int, interface{}](maxSubnetCacheSize, nil)
		if err != nil {
			log.Errorf("could not create gateway lookup: %s", err)
			return
		}

		caches = &lookupCaches{
			routeCache:  NewRouteCache(int(maxRouteCacheSize), router),
			subnetCache: subnetCache,
		}
	})

	return caches, err
}
