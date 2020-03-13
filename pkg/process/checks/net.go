package checks

import (
	"errors"
	"fmt"
	"os"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/dockerproxy"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/process/net/resolver"
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
	tracerClientID string
	networkID      string
}

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(cfg *config.AgentConfig, _ *model.SystemInfo) {
	// We use the current process PID as the system-probe client ID
	c.tracerClientID = fmt.Sprintf("%d", os.Getpid())

	// Calling the remote tracer will cause it to initialize and check connectivity
	net.SetSystemProbeSocketPath(cfg.SystemProbeSocketPath)
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
func (c *ConnectionsCheck) Name() string { return "connections" }

// Endpoint returns the endpoint where this check is submitted.
func (c *ConnectionsCheck) Endpoint() string { return "/api/v1/collector" }

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
		if err == network.ErrNotImplemented || err == ErrTracerStillNotInitialized {
			return nil, nil
		}
		return nil, err
	}

	// Filter out (in-place) connection data associated with docker-proxy
	dockerproxy.NewFilter().Filter(conns)
	// Resolve the Raddr side of connections for local containers
	LocalResolver.Resolve(conns)

	log.Debugf("collected connections in %s", time.Since(start))
	return batchConnections(cfg, groupID, c.enrichConnections(conns.Conns), conns.Dns, c.networkID), nil
}

func (c *ConnectionsCheck) getConnections() (*model.Connections, error) {
	tu, err := net.GetRemoteSystemProbeUtil()
	if err != nil {
		if net.ShouldLogTracerUtilError() {
			return nil, err
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

// Connections are split up into a chunks of a configured size conns per message to limit the message size on intake.
func batchConnections(
	cfg *config.AgentConfig,
	groupID int32,
	cxs []*model.Connection,
	dns map[string]*model.DNSEntry,
	networkID string,
) []model.MessageBody {
	groupSize := groupSize(len(cxs), cfg.MaxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	dnsEncoder := model.NewV1DNSEncoder()

	for len(cxs) > 0 {
		batchSize := min(cfg.MaxConnsPerMessage, len(cxs))
		batchConns := cxs[:batchSize] // Connections for this particular batch

		batchDNS := make(map[string]*model.DNSEntry)
		for _, c := range batchConns { // We only want to include DNS entries relevant to this batch of connections
			if entries, ok := dns[c.Raddr.Ip]; ok {
				batchDNS[c.Raddr.Ip] = entries
			}
		}

		// Get the container and process relationship from either the process or container checks
		ctrIDForPID := getCtrIDsByPIDs(connectionPIDs(batchConns))

		batches = append(batches, &model.CollectorConnections{
			HostName:        cfg.HostName,
			NetworkId:       networkID,
			Connections:     batchConns,
			GroupId:         groupID,
			GroupSize:       groupSize,
			ContainerForPid: ctrIDForPID,
			EncodedDNS:      dnsEncoder.Encode(batchDNS),
		})
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

// getCtrIDsByPIDs will fetch container id and pid relationship from either process check or container check, depend on which one is enabled and ran
func getCtrIDsByPIDs(pids []int32) map[int32]string {
	// process check is never run, use container check instead
	if Process.lastRun.IsZero() {
		return Container.filterCtrIDsByPIDs(pids)
	}
	return Process.filterCtrIDsByPIDs(pids)
}
