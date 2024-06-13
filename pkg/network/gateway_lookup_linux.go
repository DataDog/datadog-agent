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
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/vishvananda/netns"

	telemetryComponent "github.com/DataDog/datadog-agent/comp/core/telemetry"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultMaxRouteCacheSize  = uint32(math.MaxUint32)
	defaultMaxSubnetCacheSize = 1024
	gatewayLookupModuleName   = "network__gateway_lookup"
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

// gatewayLookup implements a gateway lookup
// functionality for linux
type gatewayLookup struct {
	rootNetNs   netns.NsHandle
	routeCache  RouteCache
	subnetCache *simplelru.LRU[int, interface{}] // interface index to subnet cache
}

type cloudProvider interface {
	IsAWS() bool
}

// SubnetForHwAddrFunc is exported for tracer testing purposes
var SubnetForHwAddrFunc func(net.HardwareAddr) (Subnet, error)

// Cloud is exported for tracer testing purposes
var Cloud cloudProvider

func init() {
	Cloud = &cloudProviderImpl{}
	SubnetForHwAddrFunc = ec2SubnetForHardwareAddr
}

func gwLookupEnabled() bool {
	// only enabled on AWS currently
	return Cloud.IsAWS() && ddconfig.IsCloudProviderEnabled(ec2.CloudProviderName)
}

// NewGatewayLookup creates a new instance of a gateway lookup using
// a given root network namespace and a size for the route cache
func NewGatewayLookup(rootNsLookup nsLookupFunc, maxRouteCacheSize uint32, telemetryComp telemetryComponent.Component) GatewayLookup {
	if !gwLookupEnabled() {
		return nil
	}

	rootNetNs, err := rootNsLookup()
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	gl := &gatewayLookup{
		rootNetNs: rootNetNs,
	}

	router, err := NewNetlinkRouter(rootNetNs)
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	routeCacheSize := defaultMaxRouteCacheSize
	if maxRouteCacheSize <= routeCacheSize {
		routeCacheSize = maxRouteCacheSize
	} else {
		log.Warnf("using truncated route cache size of %d instead of %d", routeCacheSize, defaultMaxRouteCacheSize)
	}

	gl.subnetCache, _ = simplelru.NewLRU[int, interface{}](int(routeCacheSize), nil)
	gl.routeCache = NewRouteCache(telemetryComp, int(routeCacheSize), router)
	return gl
}

// Lookup performs a gateway lookup for connection stats
func (g *gatewayLookup) Lookup(cs *ConnectionStats) *Via {
	dest := cs.Dest
	if cs.IPTranslation != nil {
		dest = cs.IPTranslation.ReplSrcIP
	}

	return g.LookupWithIPs(cs.Source, dest, cs.NetNS)
}

// LookupWithIPs performs a gateway lookup given the
// source, destination, and namespace
func (g *gatewayLookup) LookupWithIPs(source util.Address, dest util.Address, netns uint32) *Via {
	r, ok := g.routeCache.Get(source, dest, netns)
	if !ok {
		return nil
	}

	// if there is no gateway, we don't need to add subnet info
	// for gateway resolution in the backend
	if r.Gateway.IsZero() || r.Gateway.IsUnspecified() {
		return nil
	}

	gatewayLookupTelemetry.subnetCacheLookups.Inc()
	v, ok := g.subnetCache.Get(r.IfIndex)
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
				g.subnetCache.Add(r.IfIndex, time.Now().Add(1*time.Minute))
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			if ifi.Flags&net.FlagLoopback != 0 {
				// negative cache loopback interfaces
				g.subnetCache.Add(r.IfIndex, nil)
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			gatewayLookupTelemetry.subnetLookups.Inc()
			if s, err = SubnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
				log.Debugf("error getting subnet info for interface index %d: %s", r.IfIndex, err)

				// cache an empty result so that we don't keep hitting the
				// ec2 metadata endpoint for this interface
				if errors.IsTimeout(err) {
					// retry after a minute if we timed out
					g.subnetCache.Add(r.IfIndex, time.Now().Add(time.Minute))
					gatewayLookupTelemetry.subnetLookupErrors.Inc("timeout")
				} else {
					g.subnetCache.Add(r.IfIndex, nil)
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
		g.subnetCache.Add(r.IfIndex, via)
		gatewayLookupTelemetry.subnetCacheSize.Inc()
		v = via
	} else if v == nil {
		return nil
	}

	switch cv := v.(type) {
	case time.Time:
		if time.Now().After(cv) {
			g.subnetCache.Remove(r.IfIndex)
			gatewayLookupTelemetry.subnetCacheSize.Dec()
		}
		return nil
	case *Via:
		return cv
	default:
		return nil
	}
}

// Close cleans up resources allocated
// by this struct
func (g *gatewayLookup) Close() {
	g.rootNetNs.Close()
	g.routeCache.Close()
	g.purge()
}

func (g *gatewayLookup) purge() {
	g.subnetCache.Purge()
	gatewayLookupTelemetry.subnetCacheSize.Set(0)
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
