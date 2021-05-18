package checks

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync/atomic"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/dockerproxy"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
	procutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// Connections is a singleton ConnectionsCheck.
	Connections = &ConnectionsCheck{}

	// LocalResolver is a singleton LocalResolver
	LocalResolver = &resolver.LocalResolver{}

	// ErrTracerStillNotInitialized signals that the tracer is _still_ not ready, so we shouldn't log additional errors
	ErrTracerStillNotInitialized = errors.New("remote tracer is still not initialized")
)

// ConnectionsCheck collects statistics about live TCP and UDP connections.
type ConnectionsCheck struct {
	tracerClientID         string
	networkID              string
	notInitializedLogLimit *procutil.LogLimit
	lastTelemetry          *model.CollectorConnectionsTelemetry
	// store the last collection result by PID, currently used to populate network data for processes
	// it's in format map[int32][]*model.Connections
	lastConnsByPID atomic.Value
}

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(cfg *config.AgentConfig, _ *model.SystemInfo) {
	c.notInitializedLogLimit = procutil.NewLogLimit(1, time.Minute*10)

	// We use the current process PID as the system-probe client ID
	c.tracerClientID = fmt.Sprintf("%d", os.Getpid())

	// Calling the remote tracer will cause it to initialize and check connectivity
	net.SetSystemProbePath(cfg.SystemProbeAddress)
	_, _ = net.GetRemoteSystemProbeUtil()

	networkID, err := util.GetNetworkID()
	if err != nil {
		log.Infof("no network ID detected: %s", err)
	}
	c.networkID = networkID

	// Run the check one time on init to register the client on the system probe
	_, _ = c.Run(cfg, 0)
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

	connTel := c.diffTelemetry(conns.ConnTelemetry)

	c.lastConnsByPID.Store(getConnectionsByPID(conns))

	log.Debugf("collected connections in %s", time.Since(start))
	return batchConnections(cfg, groupID, c.enrichConnections(conns.Conns), conns.Dns, c.networkID, connTel, conns.CompilationTelemetryByAsset, conns.Domains, conns.Routes), nil
}

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

func (c *ConnectionsCheck) diffTelemetry(tel *model.ConnectionsTelemetry) *model.CollectorConnectionsTelemetry {
	if tel == nil {
		return nil
	}
	// only save but do not report the first collected telemetry to prevent reporting full monotonic values.
	if c.lastTelemetry == nil {
		c.lastTelemetry = &model.CollectorConnectionsTelemetry{}
		c.saveTelemetry(tel)
		return nil
	}

	cct := &model.CollectorConnectionsTelemetry{
		KprobesTriggered:          tel.MonotonicKprobesTriggered - c.lastTelemetry.KprobesTriggered,
		KprobesMissed:             tel.MonotonicKprobesMissed - c.lastTelemetry.KprobesMissed,
		ConntrackRegisters:        tel.MonotonicConntrackRegisters - c.lastTelemetry.ConntrackRegisters,
		ConntrackRegistersDropped: tel.MonotonicConntrackRegistersDropped - c.lastTelemetry.ConntrackRegistersDropped,
		DnsPacketsProcessed:       tel.MonotonicDnsPacketsProcessed - c.lastTelemetry.DnsPacketsProcessed,
		ConnsClosed:               tel.MonotonicConnsClosed - c.lastTelemetry.ConnsClosed,
		ConnsBpfMapSize:           tel.ConnsBpfMapSize,
		UdpSendsProcessed:         tel.MonotonicUdpSendsProcessed - c.lastTelemetry.UdpSendsProcessed,
		UdpSendsMissed:            tel.MonotonicUdpSendsMissed - c.lastTelemetry.UdpSendsMissed,
		ConntrackSamplingPercent:  tel.ConntrackSamplingPercent,
		DnsStatsDropped:           tel.DnsStatsDropped,
	}
	c.saveTelemetry(tel)
	return cct
}

func (c *ConnectionsCheck) saveTelemetry(tel *model.ConnectionsTelemetry) {
	if tel == nil || c.lastTelemetry == nil {
		return
	}

	c.lastTelemetry.KprobesTriggered = tel.MonotonicKprobesTriggered
	c.lastTelemetry.KprobesMissed = tel.MonotonicKprobesMissed
	c.lastTelemetry.ConntrackRegisters = tel.MonotonicConntrackRegisters
	c.lastTelemetry.ConntrackRegistersDropped = tel.MonotonicConntrackRegistersDropped
	c.lastTelemetry.DnsPacketsProcessed = tel.MonotonicDnsPacketsProcessed
	c.lastTelemetry.ConnsClosed = tel.MonotonicConnsClosed
	c.lastTelemetry.UdpSendsProcessed = tel.MonotonicUdpSendsProcessed
	c.lastTelemetry.UdpSendsMissed = tel.MonotonicUdpSendsMissed
	c.lastTelemetry.DnsStatsDropped = tel.DnsStatsDropped
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

// Connections are split up into a chunks of a configured size conns per message to limit the message size on intake.
func batchConnections(
	cfg *config.AgentConfig,
	groupID int32,
	cxs []*model.Connection,
	dns map[string]*model.DNSEntry,
	networkID string,
	connTelemetry *model.CollectorConnectionsTelemetry,
	compilationTelemetry map[string]*model.RuntimeCompilationTelemetry,
	domains []string,
	routes []*model.Route,
) []model.MessageBody {
	groupSize := groupSize(len(cxs), cfg.MaxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	dnsEncoder := model.NewV1DNSEncoder()

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
		batchDNS := make(map[string]*model.DNSEntry)
		domainIndices := make(map[int32]struct{})
		for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
			if entries, ok := dns[c.Raddr.Ip]; ok {
				batchDNS[c.Raddr.Ip] = entries
			}

			if c.Laddr.ContainerId != "" {
				ctrIDForPID[c.Pid] = c.Laddr.ContainerId
			}
			for d := range c.DnsStatsByDomain {
				domainIndices[d] = struct{}{}
			}
		}

		// We want to keep the length of the domains array same so that the pointers in DnsStatsByDomain remain valid
		// For absent entries, we simply use an empty string to cut down on storage.
		batchDomains := make([]string, len(domains))
		for i, domain := range domains {
			if _, ok := domainIndices[int32(i)]; ok {
				batchDomains[i] = domain
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

		cc := &model.CollectorConnections{
			HostName:          cfg.HostName,
			NetworkId:         networkID,
			Connections:       batchConns,
			GroupId:           groupID,
			GroupSize:         groupSize,
			ContainerForPid:   ctrIDForPID,
			EncodedDNS:        dnsEncoder.Encode(batchDNS),
			ContainerHostType: cfg.ContainerHostType,
			Domains:           batchDomains,
			Routes:            batchRoutes,
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
			cc.ConnTelemetry = connTelemetry
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
