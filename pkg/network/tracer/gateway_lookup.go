// +build linux_bpf

package tracer

import (
	"net"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
	"github.com/hashicorp/golang-lru/simplelru"
)

const maxRouteCacheSize = int(^uint(0) >> 1) // max int
const maxSubnetCacheSize = 1024

type gatewayLookup struct {
	routeCache          network.RouteCache
	subnetCache         *simplelru.LRU // interface index to subnet cache
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

func newGatewayLookup(config *config.Config, m *manager.Manager) *gatewayLookup {
	if !gwLookupEnabled(config) {
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

	lru, _ := simplelru.NewLRU(maxSubnetCacheSize, nil)
	return &gatewayLookup{
		subnetCache:         lru,
		routeCache:          network.NewRouteCache(routeCacheSize, router),
		subnetForHwAddrFunc: ec2SubnetForHardwareAddr,
	}
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
	if util.NetIPFromAddress(r.Gateway).IsUnspecified() {
		return nil
	}

	v, ok := g.subnetCache.Get(r.IfIndex)
	if !ok {
		ifi, err := net.InterfaceByIndex(r.IfIndex)
		if err != nil {
			log.Errorf("error getting interface for interface index %d: %s", r.IfIndex, err)
			return nil
		}

		if ifi.Flags&net.FlagLoopback != 0 {
			return nil
		}

		var s network.Subnet
		if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
			log.Errorf("error getting subnet info for interface index %d: %s", r.IfIndex, err)
			// cache an empty result so that we don't keep hitting the
			// ec2 metadata endpoint for this interface
			g.subnetCache.Add(r.IfIndex, nil)
			return nil
		}

		g.subnetCache.Add(r.IfIndex, s)
		v = s
	} else if v == nil {
		return nil
	}

	return &network.Via{Subnet: v.(network.Subnet)}
}

func (g *gatewayLookup) purge() {
	g.subnetCache.Purge()
}

func ec2SubnetForHardwareAddr(hwAddr net.HardwareAddr) (network.Subnet, error) {
	snet, err := ec2.GetSubnetForHardwareAddr(hwAddr)
	if err != nil {
		return network.Subnet{}, err
	}

	return network.Subnet{Alias: snet.ID}, nil
}

type cloudProviderImpl struct{}

func (cp *cloudProviderImpl) IsAWS() bool {
	return ec2.IsRunningOn()
}
