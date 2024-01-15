// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"context"
	"math"
	"net"
	"time"

	"github.com/hashicorp/golang-lru/v2/simplelru"
	"github.com/vishvananda/netns"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	maxRouteCacheSize       = uint32(math.MaxUint32)
	maxSubnetCacheSize      = 1024
	gatewayLookupModuleName = "network_tracer__gateway_lookup"
)

// Telemetry
var gatewayLookupTelemetry = struct {
	subnetCacheSize    *ebpftelemetry.StatGaugeWrapper
	subnetCacheMisses  *ebpftelemetry.StatCounterWrapper
	subnetCacheLookups *ebpftelemetry.StatCounterWrapper
	subnetLookups      *ebpftelemetry.StatCounterWrapper
	subnetLookupErrors *ebpftelemetry.StatCounterWrapper
}{
	ebpftelemetry.NewStatGaugeWrapper(gatewayLookupModuleName, "subnet_cache_size", []string{}, "Counter measuring the size of the subnet cache"),
	ebpftelemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_cache_misses", []string{}, "Counter measuring the number of subnet cache misses"),
	ebpftelemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_cache_lookups", []string{}, "Counter measuring the number of subnet cache lookups"),
	ebpftelemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_lookups", []string{}, "Counter measuring the number of subnet lookups"),
	ebpftelemetry.NewStatCounterWrapper(gatewayLookupModuleName, "subnet_lookup_errors", []string{}, "Counter measuring the number of subnet lookup errors"),
}

type gatewayLookup struct {
	procRoot            string
	rootNetNs           netns.NsHandle
	routeCache          network.RouteCache
	subnetCache         *simplelru.LRU[int, interface{}] // interface index to subnet cache
	subnetForHwAddrFunc func(net.HardwareAddr) (network.Subnet, error)
}

type cloudProvider interface {
	IsAWS() bool
}

var cloud cloudProvider

func init() {
	cloud = &cloudProviderImpl{}
}

func gwLookupEnabled(config *config.Config) bool {
	// only enabled on AWS currently
	return config.EnableGatewayLookup && cloud.IsAWS() && ddconfig.IsCloudProviderEnabled(ec2.CloudProviderName)
}

func newGatewayLookup(config *config.Config) *gatewayLookup {
	if !gwLookupEnabled(config) {
		return nil
	}

	ns, err := config.GetRootNetNs()
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	gl := &gatewayLookup{
		procRoot:            config.ProcRoot,
		rootNetNs:           ns,
		subnetForHwAddrFunc: ec2SubnetForHardwareAddr,
	}

	router, err := network.NewNetlinkRouter(config)
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	routeCacheSize := maxRouteCacheSize
	if config.MaxTrackedConnections <= maxRouteCacheSize {
		routeCacheSize = config.MaxTrackedConnections
	} else {
		log.Warnf("using truncated route cache size of %d instead of %d", routeCacheSize, config.MaxTrackedConnections)
	}

	gl.subnetCache, _ = simplelru.NewLRU[int, interface{}](maxSubnetCacheSize, nil)
	gl.routeCache = network.NewRouteCache(int(routeCacheSize), router)
	return gl
}

func (g *gatewayLookup) Lookup(cs *network.ConnectionStats) *network.Via {
	dest := cs.Dest
	if cs.IPTranslation != nil {
		dest = cs.IPTranslation.ReplSrcIP
	}

	r, ok := g.routeCache.Get(cs.Source, dest, cs.NetNS)
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

		var s network.Subnet
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
			if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
				gatewayLookupTelemetry.subnetLookupErrors.Inc()
				log.Debugf("error getting subnet info for interface index %d: %s", r.IfIndex, err)

				// cache an empty result so that we don't keep hitting the
				// ec2 metadata endpoint for this interface
				g.subnetCache.Add(r.IfIndex, nil)
				gatewayLookupTelemetry.subnetCacheSize.Inc()
				return err
			}

			return nil
		})

		if err != nil {
			return nil
		}

		via := &network.Via{Subnet: s}
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
	case *network.Via:
		return cv
	default:
		return nil
	}
}

func (g *gatewayLookup) Close() {
	g.rootNetNs.Close()
	g.routeCache.Close()
	g.purge()
}

func (g *gatewayLookup) purge() {
	g.subnetCache.Purge()
	gatewayLookupTelemetry.subnetCacheSize.Set(0)
}

func ec2SubnetForHardwareAddr(hwAddr net.HardwareAddr) (network.Subnet, error) {
	snet, err := ec2.GetSubnetForHardwareAddr(context.TODO(), hwAddr)
	if err != nil {
		return network.Subnet{}, err
	}

	return network.Subnet{Alias: snet.ID}, nil
}

type cloudProviderImpl struct{}

func (cp *cloudProviderImpl) IsAWS() bool {
	return ec2.IsRunningOn(context.TODO())
}
