// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"sort"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/dockerproxy"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
	procutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/cloudproviders"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/atomic"
)

var (
	// Connections is a singleton ConnectionsCheck.
	Connections = &ConnectionsCheck{
		lastConnsByPID: &atomic.Value{},
	}

	// LocalResolver is a singleton LocalResolver
	LocalResolver = &resolver.LocalResolver{}

	// ErrTracerStillNotInitialized signals that the tracer is _still_ not ready, so we shouldn't log additional errors
	ErrTracerStillNotInitialized = errors.New("remote tracer is still not initialized")

	// ProcessAgentClientID process-agent unique ID
	ProcessAgentClientID = "process-agent-unique-id"
)

// ConnectionsCheck collects statistics about live TCP and UDP connections.
type ConnectionsCheck struct {
	tracerClientID         string
	networkID              string
	notInitializedLogLimit *procutil.LogLimit
	// store the last collection result by PID, currently used to populate network data for processes
	// it's in format map[int32][]*model.Connections
	lastConnsByPID *atomic.Value
}

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(cfg *config.AgentConfig, _ *model.SystemInfo) {
	c.notInitializedLogLimit = procutil.NewLogLimit(1, time.Minute*10)

	// We use the current process PID as the system-probe client ID
	c.tracerClientID = ProcessAgentClientID

	// Calling the remote tracer will cause it to initialize and check connectivity
	net.SetSystemProbePath(cfg.SystemProbeAddress)
	tu, err := net.GetRemoteSystemProbeUtil()

	if err != nil {
		log.Warnf("could not initiate connection with system probe: %s", err)
	} else {
		// Register process agent as a system probe's client
		// This ensures we start recording data from now to the first call to `Run`
		err = tu.Register(c.tracerClientID)
		if err != nil {
			log.Warnf("could not register process-agent to system-probe: %s", err)
		}
	}

	networkID, err := cloudproviders.GetNetworkID(context.TODO())
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID
}

// Name returns the name of the ConnectionsCheck.
func (c *ConnectionsCheck) Name() string { return config.ConnectionsCheckName }

// RealTime indicates if this check only runs in real-time mode.
func (c *ConnectionsCheck) RealTime() bool { return false }

// Run runs the ConnectionsCheck to collect the live TCP connections on the
// system. Currently only linux systems are supported as eBPF is used to gather
// this information. For each connection we'll return a `model.Connection`
// that will be bundled up into a `CollectorConnections`.
// See agent.proto for the schema of the message and models.
func (c *ConnectionsCheck) Run(cfg *config.AgentConfig, groupID int32) ([]model.MessageBody, error) {
	start := time.Now()

	conns, err := c.getConnections()
	if err != nil {
		// If the tracer is not initialized, or still not initialized, then we want to exit without error'ing
		if err == ebpf.ErrNotImplemented || err == ErrTracerStillNotInitialized {
			return nil, nil
		}
		return nil, err
	}

	// Filter out (in-place) connection data associated with docker-proxy
	dockerproxy.NewFilter().Filter(conns)
	// Resolve the Raddr side of connections for local containers
	LocalResolver.Resolve(conns)

	c.lastConnsByPID.Store(getConnectionsByPID(conns))

	log.Debugf("collected connections in %s", time.Since(start))
	return batchConnections(cfg, groupID, c.enrichConnections(conns.Conns), conns.Dns, c.networkID, conns.ConnTelemetryMap, conns.CompilationTelemetryByAsset, conns.Domains, conns.Routes, conns.Tags, conns.AgentConfiguration), nil
}

// Cleanup frees any resource held by the ConnectionsCheck before the agent exits
func (c *ConnectionsCheck) Cleanup() {}

func (c *ConnectionsCheck) getConnections() (*model.Connections, error) {
	tu, err := net.GetRemoteSystemProbeUtil()
	if err != nil {
		if c.notInitializedLogLimit.ShouldLog() {
			log.Warnf("could not initialize system-probe connection: %v (will only log every 10 minutes)", err)
		}
		return nil, ErrTracerStillNotInitialized
	}
	return tu.GetConnections(c.tracerClientID)
}

func (c *ConnectionsCheck) enrichConnections(conns []*model.Connection) []*model.Connection {
	// Process create-times required to construct unique process hash keys on the backend
	createTimeForPID := Process.createTimesforPIDs(connectionPIDs(conns))
	for _, conn := range conns {
		if _, ok := createTimeForPID[conn.Pid]; !ok {
			createTimeForPID[conn.Pid] = 0
		}

		conn.PidCreateTime = createTimeForPID[conn.Pid]
	}
	return conns
}

func (c *ConnectionsCheck) getLastConnectionsByPID() map[int32][]*model.Connection {
	if result := c.lastConnsByPID.Load(); result != nil {
		return result.(map[int32][]*model.Connection)
	}
	return nil
}

// getConnectionsByPID groups a list of connection objects by PID
func getConnectionsByPID(conns *model.Connections) map[int32][]*model.Connection {
	result := make(map[int32][]*model.Connection)
	for _, conn := range conns.Conns {
		result[conn.Pid] = append(result[conn.Pid], conn)
	}
	return result
}

func convertDNSEntry(dnstable map[string]*model.DNSDatabaseEntry, namemap map[string]int32, namedb *[]string, ip string, entry *model.DNSEntry) {
	dbentry := &model.DNSDatabaseEntry{
		NameOffsets: make([]int32, 0, len(entry.Names)),
	}
	for _, name := range entry.Names {
		// at this point, the NameOffsets slice is actually a slice of indices into
		// the name slice.  It will be converted prior to encoding.
		if idx, ok := namemap[name]; ok {
			dbentry.NameOffsets = append(dbentry.NameOffsets, idx)
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, name)
			namemap[name] = dblen
			dbentry.NameOffsets = append(dbentry.NameOffsets, dblen)
		}

	}
	dnstable[ip] = dbentry
}

func remapDNSStatsByDomain(c *model.Connection, namemap map[string]int32, namedb *[]string, dnslist []string) {
	old := c.DnsStatsByDomain
	if old == nil || len(old) == 0 {
		return
	}
	c.DnsStatsByDomain = make(map[int32]*model.DNSStats)
	for key, val := range old {
		// key is the index into the old array (dnslist)
		domainstr := dnslist[key]
		if idx, ok := namemap[domainstr]; ok {
			c.DnsStatsByDomain[idx] = val
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, domainstr)
			namemap[domainstr] = dblen
			c.DnsStatsByDomain[dblen] = val
		}
	}
}

func remapDNSStatsByDomainByQueryType(c *model.Connection, namemap map[string]int32, namedb *[]string, dnslist []string) {
	old := c.DnsStatsByDomainByQueryType
	c.DnsStatsByDomainByQueryType = make(map[int32]*model.DNSStatsByQueryType)
	for key, val := range old {
		// key is the index into the old array (dnslist)
		domainstr := dnslist[key]
		if idx, ok := namemap[domainstr]; ok {
			c.DnsStatsByDomainByQueryType[idx] = val
		} else {
			dblen := int32(len(*namedb))
			*namedb = append(*namedb, domainstr)
			namemap[domainstr] = dblen
			c.DnsStatsByDomainByQueryType[dblen] = val
		}
	}

}

func remapDNSStatsByOffset(c *model.Connection, indexToOffset []int32) {
	oldByDomain := c.DnsStatsByDomain
	oldByDomainByQueryType := c.DnsStatsByDomainByQueryType

	c.DnsStatsByDomainOffsetByQueryType = make(map[int32]*model.DNSStatsByQueryType)

	// first, walk the stats by domain.  Put them in by query type 'A`
	for key, val := range oldByDomain {
		off := indexToOffset[key]
		if _, ok := c.DnsStatsByDomainOffsetByQueryType[off]; !ok {
			c.DnsStatsByDomainOffsetByQueryType[off] = &model.DNSStatsByQueryType{}
			c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType = make(map[int32]*model.DNSStats)
		}
		c.DnsStatsByDomainOffsetByQueryType[off].DnsStatsByQueryType[int32(dns.TypeA)] = val
	}
	for key, val := range oldByDomainByQueryType {
		off := indexToOffset[key]
		c.DnsStatsByDomainOffsetByQueryType[off] = val
	}
	c.DnsStatsByDomain = nil
	c.DnsStatsByDomainByQueryType = nil
}

// Connections are split up into a chunks of a configured size conns per message to limit the message size on intake.
func batchConnections(
	cfg *config.AgentConfig,
	groupID int32,
	cxs []*model.Connection,
	dns map[string]*model.DNSEntry,
	networkID string,
	connTelemetryMap map[string]int64,
	compilationTelemetry map[string]*model.RuntimeCompilationTelemetry,
	domains []string,
	routes []*model.Route,
	tags []string,
	agentCfg *model.AgentConfiguration,
) []model.MessageBody {
	groupSize := groupSize(len(cxs), cfg.MaxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	dnsEncoder := model.NewV2DNSEncoder()

	if len(cxs) > cfg.MaxConnsPerMessage {
		// Sort connections by remote IP/PID for more efficient resolution
		sort.Slice(cxs, func(i, j int) bool {
			if cxs[i].Raddr.Ip != cxs[j].Raddr.Ip {
				return cxs[i].Raddr.Ip < cxs[j].Raddr.Ip
			}
			return cxs[i].Pid < cxs[j].Pid
		})
	}

	for len(cxs) > 0 {
		batchSize := min(cfg.MaxConnsPerMessage, len(cxs))
		batchConns := cxs[:batchSize] // Connections for this particular batch

		ctrIDForPID := make(map[int32]string)
		batchDNS := make(map[string]*model.DNSDatabaseEntry)
		namemap := make(map[string]int32)
		namedb := make([]string, 0)

		tagsEncoder := model.NewV2TagEncoder()

		for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
			if entries, ok := dns[c.Raddr.Ip]; ok {
				if _, present := batchDNS[c.Raddr.Ip]; !present {
					// first, walks through and converts entries of type DNSEntry to DNSDatabaseEntry,
					// so that we're always sending the same (newer) type.
					convertDNSEntry(batchDNS, namemap, &namedb, c.Raddr.Ip, entries)
				}
			}

			if c.Laddr.ContainerId != "" {
				ctrIDForPID[c.Pid] = c.Laddr.ContainerId
			}

			// remap functions create a new map; the map is by string _index_ (not offset)
			// in the namedb.  Each unique string should only occur once.
			remapDNSStatsByDomain(c, namemap, &namedb, domains)
			remapDNSStatsByDomainByQueryType(c, namemap, &namedb, domains)

			// tags remap
			if len(c.Tags) > 0 {
				var tagsStr []string
				for _, t := range c.Tags {
					tagsStr = append(tagsStr, tags[t])
				}
				c.Tags = nil
				c.TagsIdx = int32(tagsEncoder.Encode(tagsStr))
			} else {
				c.TagsIdx = -1
			}

		}

		// remap route indices
		// map of old index to new index
		newRouteIndices := make(map[int32]int32)
		var batchRoutes []*model.Route
		for _, c := range batchConns {
			if c.RouteIdx < 0 {
				continue
			}
			if i, ok := newRouteIndices[c.RouteIdx]; ok {
				c.RouteIdx = i
				continue
			}

			new := int32(len(newRouteIndices))
			newRouteIndices[c.RouteIdx] = new
			batchRoutes = append(batchRoutes, routes[c.RouteIdx])
			c.RouteIdx = new
		}

		// EncodeDomainDatabase will take the namedb (a simple slice of strings with each unique
		// domain string) and convert it into a buffer of all of the strings.
		// indexToOffset contains the map from the string index to where it occurs in the encodedNameDb
		var mappedDNSLookups []byte
		encodedNameDb, indexToOffset, err := dnsEncoder.EncodeDomainDatabase(namedb)
		if err != nil {
			encodedNameDb = nil
			// since we were unable to properly encode the indexToOffet map, the
			// rest of the maps will now be unreadable by the back-end.  Just clear them
			for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
				c.DnsStatsByDomain = nil
				c.DnsStatsByDomainByQueryType = nil
				c.DnsStatsByDomainOffsetByQueryType = nil
			}
		} else {

			// Now we have all available information.  EncodeMapped with take the string indices
			// that are used, and encode (using the indexToOffset array) the offset into the buffer
			// this way individual strings can be directly accessed on decode.
			mappedDNSLookups, err = dnsEncoder.EncodeMapped(batchDNS, indexToOffset)
			if err != nil {
				mappedDNSLookups = nil
			}
			for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
				remapDNSStatsByOffset(c, indexToOffset)
			}
		}
		cc := &model.CollectorConnections{
			AgentConfiguration:     agentCfg,
			HostName:               cfg.HostName,
			NetworkId:              networkID,
			Connections:            batchConns,
			GroupId:                groupID,
			GroupSize:              groupSize,
			ContainerForPid:        ctrIDForPID,
			EncodedDomainDatabase:  encodedNameDb,
			EncodedDnsLookups:      mappedDNSLookups,
			ContainerHostType:      cfg.ContainerHostType,
			Routes:                 batchRoutes,
			EncodedConnectionsTags: tagsEncoder.Buffer(),
		}

		// Add OS telemetry
		if hostInfo := host.GetStatusInformation(); hostInfo != nil {
			cc.KernelVersion = hostInfo.KernelVersion
			cc.Architecture = hostInfo.KernelArch
			cc.Platform = hostInfo.Platform
			cc.PlatformVersion = hostInfo.PlatformVersion
		}

		// only add the telemetry to the first message to prevent double counting
		if len(batches) == 0 {
			cc.ConnTelemetryMap = connTelemetryMap
			cc.CompilationTelemetryByAsset = compilationTelemetry
		}
		batches = append(batches, cc)

		cxs = cxs[batchSize:]
	}
	return batches
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func groupSize(total, maxBatchSize int) int32 {
	groupSize := total / maxBatchSize
	if total%maxBatchSize > 0 {
		groupSize++
	}
	return int32(groupSize)
}

func connectionPIDs(conns []*model.Connection) []int32 {
	ps := make(map[int32]struct{})
	for _, c := range conns {
		ps[c.Pid] = struct{}{}
	}

	pids := make([]int32, 0, len(ps))
	for pid := range ps {
		pids = append(pids, pid)
	}
	return pids
}
