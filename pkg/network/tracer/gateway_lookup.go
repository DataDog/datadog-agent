// +build linux_bpf

package tracer

import (
	"net"
	"time"

	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
)

const maxRouteCacheSize = int(^uint(0) >> 1) // max int

type gatewayLookup struct {
	routeCache          network.RouteCache
	subnetCache         map[int]network.Subnet // interface index to subnet map
	subnetForHwAddrFunc func(net.HardwareAddr) (network.Subnet, error)

	logLimiter *util.LogLimit
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

	return &gatewayLookup{
		subnetCache:         make(map[int]network.Subnet),
		routeCache:          network.NewRouteCache(routeCacheSize, router),
		subnetForHwAddrFunc: ec2SubnetForHardwareAddr,
		logLimiter:          util.NewLogLimit(10, 10*time.Minute),
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

	s, ok := g.subnetCache[r.IfIndex]
	if !ok {
		ifi, err := net.InterfaceByIndex(r.IfIndex)
		if err != nil {
			log.Errorf("error getting interface for interface index %d: %s", r.IfIndex, err)
			return nil
		}

		if ifi.Flags&net.FlagLoopback != 0 {
			return nil
		}

		if s, err = g.subnetForHwAddrFunc(ifi.HardwareAddr); err != nil {
			if g.logLimiter.ShouldLog() {
				log.Errorf("error getting subnet info for interface index %d: %s", r.IfIndex, err)
			}
			return nil
		}

		g.subnetCache[r.IfIndex] = s
	}

	return &network.Via{Subnet: s}
}

func (g *gatewayLookup) purge() {
	g.subnetCache = make(map[int]network.Subnet)
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
