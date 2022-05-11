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
	"sync/atomic"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/stats"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/hashicorp/golang-lru/simplelru"
)

const maxRouteCacheSize = int(^uint(0) >> 1) // max int
const maxSubnetCacheSize = 1024

type gatewayLookup struct {
	procRoot            string
	routeCache          network.RouteCache
	subnetCache         *simplelru.LRU // interface index to subnet cache
	subnetForHwAddrFunc func(net.HardwareAddr) (network.Subnet, error)

	// stats
	subnetCacheSize    uint64 `stats:"atomic"`
	subnetCacheMisses  uint64 `stats:"atomic"`
	subnetCacheLookups uint64 `stats:"atomic"`
	subnetLookups      uint64 `stats:"atomic"`
	subnetLookupErrors uint64 `stats:"atomic"`

	statsReporter stats.Reporter
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

	gl := &gatewayLookup{procRoot: config.ProcRoot, subnetForHwAddrFunc: ec2SubnetForHardwareAddr}

	var err error
	gl.statsReporter, err = stats.NewReporter(gl)
	if err != nil {
		log.Errorf("could not create stats reporter for gateway lookup: %s", err)
		return nil
	}

	router, err := network.NewNetlinkRouter(config.ProcRoot)
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

	atomic.AddUint64(&g.subnetCacheLookups, 1)
	v, ok := g.subnetCache.Get(r.IfIndex)
	if !ok {
		atomic.AddUint64(&g.subnetCacheMisses, 1)

		var s network.Subnet
		var err error
		err = util.WithRootNS(g.procRoot, func() error {
			var ifi *net.Interface
			ifi, err = net.InterfaceByIndex(r.IfIndex)
			if err != nil {
				log.Errorf("error getting interface for interface index %d: %s", r.IfIndex, err)
				// negative cache for 1 minute
				g.subnetCache.Add(r.IfIndex, time.Now().Add(1*time.Minute))
				atomic.AddUint64(&g.subnetCacheSize, 1)
				return err
			}

			if ifi.Flags&net.FlagLoopback != 0 {
				// negative cache loopback interfaces
				g.subnetCache.Add(r.IfIndex, nil)
				atomic.AddUint64(&g.subnetCacheSize, 1)
				return err
			}

			atomic.AddUint64(&g.subnetLookups, 1)
			if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
				atomic.AddUint64(&g.subnetLookupErrors, 1)
				log.Errorf("error getting subnet info for interface index %d: %s", r.IfIndex, err)

				// cache an empty result so that we don't keep hitting the
				// ec2 metadata endpoint for this interface
				g.subnetCache.Add(r.IfIndex, nil)
				atomic.AddUint64(&g.subnetCacheSize, 1)
				return err
			}

			return nil
		})

		if err != nil {
			return nil
		}

		via := &network.Via{Subnet: s}
		g.subnetCache.Add(r.IfIndex, via)
		atomic.AddUint64(&g.subnetCacheSize, 1)
		v = via
	} else if v == nil {
		return nil
	}

	switch cv := v.(type) {
	case time.Time:
		if time.Now().After(cv) {
			g.subnetCache.Remove(r.IfIndex)
			atomic.AddUint64(&g.subnetCacheSize, ^uint64(0))
		}
		return nil
	case *network.Via:
		return cv
	default:
		return nil
	}
}

func (g *gatewayLookup) GetStats() map[string]interface{} {
	report := g.statsReporter.Report()
	report["route_cache"] = g.routeCache.GetStats()
	return report
}

func (g *gatewayLookup) purge() {
	g.subnetCache.Purge()
	atomic.StoreUint64(&g.subnetCacheSize, 0)
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
