// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"context"
	"net"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
	"github.com/vishvananda/netns"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const maxRouteCacheSize = int(^uint(0) >> 1) // max int
const maxSubnetCacheSize = 1024

type gatewayLookup struct {
	procRoot            string
	rootNetNs           netns.NsHandle
	routeCache          network.RouteCache
	subnetCache         *simplelru.LRU // interface index to subnet cache
	subnetForHwAddrFunc func(net.HardwareAddr) (network.Subnet, error)

	// stats
	subnetCacheSize    telemetry.StatGaugeWrapper
	subnetCacheMisses  telemetry.StatGaugeWrapper
	subnetCacheLookups telemetry.StatGaugeWrapper
	subnetLookups      telemetry.StatGaugeWrapper
	subnetLookupErrors telemetry.StatGaugeWrapper
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
		subnetCacheSize:     telemetry.NewStatGaugeWrapper("gateway_lookup", "subnet_cache_size", []string{}, "description"),
		subnetCacheMisses:   telemetry.NewStatGaugeWrapper("gateway_lookup", "subnet_cache_misses", []string{}, "description"),
		subnetCacheLookups:  telemetry.NewStatGaugeWrapper("gateway_lookup", "subnet_cache_lookups", []string{}, "description"),
		subnetLookups:       telemetry.NewStatGaugeWrapper("gateway_lookup", "subnet_lookups", []string{}, "description"),
		subnetLookupErrors:  telemetry.NewStatGaugeWrapper("gateway_lookup", "subnet_lookup_errors", []string{}, "description"),
	}

	router, err := network.NewNetlinkRouter(config)
	if err != nil {
		log.Errorf("could not create gateway lookup: %s", err)
		return nil
	}

	routeCacheSize := maxRouteCacheSize
	if config.MaxTrackedConnections <= uint(maxRouteCacheSize) {
		routeCacheSize = int(config.MaxTrackedConnections)
	} else {
		log.Warnf("using truncated route cache size of %d instead of %d", routeCacheSize, config.MaxTrackedConnections)
	}

	gl.subnetCache, _ = simplelru.NewLRU(maxSubnetCacheSize, nil)
	gl.routeCache = network.NewRouteCache(routeCacheSize, router)
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

	g.subnetCacheLookups.Inc()
	v, ok := g.subnetCache.Get(r.IfIndex)
	if !ok {
		g.subnetCacheMisses.Inc()

		var s network.Subnet
		var err error
		err = util.WithNS(g.rootNetNs, func() error {
			var ifi *net.Interface
			ifi, err = net.InterfaceByIndex(r.IfIndex)
			if err != nil {
				log.Errorf("error getting interface for interface index %d: %s", r.IfIndex, err)
				// negative cache for 1 minute
				g.subnetCache.Add(r.IfIndex, time.Now().Add(1*time.Minute))
				g.subnetCacheSize.Inc()
				return err
			}

			if ifi.Flags&net.FlagLoopback != 0 {
				// negative cache loopback interfaces
				g.subnetCache.Add(r.IfIndex, nil)
				g.subnetCacheSize.Inc()
				return err
			}

			g.subnetLookups.Inc()
			if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
				g.subnetLookupErrors.Inc()
				log.Errorf("error getting subnet info for interface index %d: %s", r.IfIndex, err)

				// cache an empty result so that we don't keep hitting the
				// ec2 metadata endpoint for this interface
				g.subnetCache.Add(r.IfIndex, nil)
				g.subnetCacheSize.Inc()
				return err
			}

			return nil
		})

		if err != nil {
			return nil
		}

		via := &network.Via{Subnet: s}
		g.subnetCache.Add(r.IfIndex, via)
		g.subnetCacheSize.Inc()
		v = via
	} else if v == nil {
		return nil
	}

	switch cv := v.(type) {
	case time.Time:
		if time.Now().After(cv) {
			g.subnetCache.Remove(r.IfIndex)
			g.subnetCacheSize.Dec()
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
	g.subnetCacheSize.Set(0)
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
