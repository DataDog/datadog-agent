package checks

import (
	"errors"
	"fmt"
	"os"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// Connections is a singleton ConnectionsCheck.
	Connections = &ConnectionsCheck{}

	// ErrTracerStillNotInitialized signals that the tracer is _still_ not ready, so we shouldn't log additional errors
	ErrTracerStillNotInitialized = errors.New("remote tracer is still not initialized")
)

// ConnectionsCheck collects statistics about live TCP and UDP connections.
type ConnectionsCheck struct {
	// Local system probe
	useLocalTracer bool
	localTracer    *ebpf.Tracer
	tracerClientID string
}

// Init initializes a ConnectionsCheck instance.
func (c *ConnectionsCheck) Init(cfg *config.AgentConfig, sysInfo *model.SystemInfo) {
	// We use the current process PID as the local tracer client ID
	c.tracerClientID = fmt.Sprintf("%d", os.Getpid())
	if cfg.EnableLocalSystemProbe {
		log.Info("starting system probe locally")
		c.useLocalTracer = true

		// Checking whether the current kernel version is supported by the tracer
		if supported, msg := ebpf.IsTracerSupportedByOS(cfg.ExcludedBPFLinuxVersions); !supported {
			// err is always returned when false, so the above catches the !ok case as well
			log.Warnf("system probe unsupported by OS: %s", msg)
			return
		}

		t, err := ebpf.NewTracer(config.SysProbeConfigFromConfig(cfg))
		if err != nil {
			log.Errorf("failed to create system probe: %s", err)
			return
		}

		c.localTracer = t
	} else {
		// Calling the remote tracer will cause it to initialize and check connectivity
		net.SetSystemProbeSocketPath(cfg.SystemProbeSocketPath)
		net.GetRemoteSystemProbeUtil()
	}

	// Run the check one time on init to register the client on the system probe
	c.Run(cfg, 0)
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
	// If local tracer failed to initialize, so we shouldn't be doing any checks
	if c.useLocalTracer && c.localTracer == nil {
		return nil, nil
	}

	start := time.Now()

	conns, err := c.getConnections()
	if err != nil {
		// If the tracer is not initialized, or still not initialized, then we want to exit without error'ing
		if err == ebpf.ErrNotImplemented || err == ErrTracerStillNotInitialized {
			return nil, nil
		}
		return nil, err
	}

	log.Debugf("collected connections in %s", time.Since(start))
	return batchConnections(cfg, groupID, c.enrichConnections(conns)), nil
}

func (c *ConnectionsCheck) getConnections() ([]*model.Connection, error) {
	if c.useLocalTracer { // If local tracer is set up, use that
		if c.localTracer == nil {
			return nil, fmt.Errorf("using local system probe, but no tracer was initialized")
		}
		cs, err := c.localTracer.GetActiveConnections(c.tracerClientID)
		conns := make([]*model.Connection, len(cs.Conns))
		for i, ebpfConn := range cs.Conns {
			conns[i] = encoding.FormatConnection(ebpfConn)
		}
		return conns, err
	}

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

// Connections are split up into a chunks of at most 100 connections per message to
// limit the message size on intake.
func batchConnections(cfg *config.AgentConfig, groupID int32, cxs []*model.Connection) []model.MessageBody {
	groupSize := groupSize(len(cxs), cfg.MaxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	for len(cxs) > 0 {
		batchSize := min(cfg.MaxConnsPerMessage, len(cxs))
		// get the container and process relationship from either process check or container check
		ctrIDForPID := getCtrIDsByPIDs(connectionPIDs(cxs[:batchSize]))
		batches = append(batches, &model.CollectorConnections{
			HostName:        cfg.HostName,
			Connections:     cxs[:batchSize],
			GroupId:         groupID,
			GroupSize:       groupSize,
			ContainerForPid: ctrIDForPID,
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
