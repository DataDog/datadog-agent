package checks

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/model"
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
	var err error

	if cfg.EnableLocalSystemProbe {
		log.Info("starting system probe locally")
		c.useLocalTracer = true

		// Checking whether the current kernel version is supported by the tracer
		if _, err = ebpf.IsTracerSupportedByOS(cfg.ExcludedBPFLinuxVersions); err != nil {
			// err is always returned when false, so the above catches the !ok case as well
			log.Warnf("system probe unsupported by OS: %s", err)
			return
		}

		t, err := ebpf.NewTracer(config.SysProbeConfigFromConfig(cfg))
		if err != nil {
			log.Errorf("failed to create system probe: %s", err)
			return
		}

		c.localTracer = t
		// We use the current process PID as the local tracer client ID
		c.tracerClientID = fmt.Sprintf("%d", os.Getpid())
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
	return batchConnections(cfg, groupID, c.formatConnections(conns)), nil
}

func (c *ConnectionsCheck) getConnections() ([]ebpf.ConnectionStats, error) {
	if c.useLocalTracer { // If local tracer is set up, use that
		if c.localTracer == nil {
			return nil, fmt.Errorf("using local system probe, but no tracer was initialized")
		}
		cs, err := c.localTracer.GetActiveConnections(c.tracerClientID)
		return cs.Conns, err
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

// Connections are split up into a chunks of at most 100 connections per message to
// limit the message size on intake.
func (c *ConnectionsCheck) formatConnections(conns []ebpf.ConnectionStats) []*model.Connection {
	// Process create-times required to construct unique process hash keys on the backend
	createTimeForPID := Process.createTimesforPIDs(connectionStatsPIDs(conns))

	cxs := make([]*model.Connection, 0, len(conns))
	for _, conn := range conns {
		// default creation time to ensure network connections from short-lived processes are not dropped
		if _, ok := createTimeForPID[conn.Pid]; !ok {
			createTimeForPID[conn.Pid] = 0
		}

		source, dest, ok := formatIPs(conn.Source, conn.Dest)
		if !ok {
			continue
		}

		cxs = append(cxs, &model.Connection{
			Pid:           int32(conn.Pid),
			PidCreateTime: createTimeForPID[conn.Pid],
			NetNS:         conn.NetNS,
			Family:        formatFamily(conn.Family),
			Type:          formatType(conn.Type),
			Laddr: &model.Addr{
				Ip:   source,
				Port: int32(conn.SPort),
			},
			Raddr: &model.Addr{
				Ip:   dest,
				Port: int32(conn.DPort),
			},
			TotalBytesSent:     conn.MonotonicSentBytes,
			TotalBytesReceived: conn.MonotonicRecvBytes,
			TotalRetransmits:   conn.MonotonicRetransmits,
			LastBytesSent:      conn.LastSentBytes,
			LastBytesReceived:  conn.LastRecvBytes,
			LastRetransmits:    conn.LastRetransmits,
			Direction:          formatDirection(conn.Direction),
			IpTranslation:      formatIPTranslation(conn.IPTranslation),
		})
	}
	return cxs
}

// These are written as strings via the easyjson marshaller in ebpf.Address
func formatIPs(sourceIP, destIP interface{}) (string, string, bool) {
	source, ok := sourceIP.(string)
	if !ok {
		log.Errorf("failed to cast source IP interface to string %s", sourceIP)
		return "", "", false
	}

	dest, ok := destIP.(string)
	if !ok {
		log.Errorf("failed to cast dest IP interface to string %s", destIP)
		return "", "", false
	}
	return source, dest, true
}

func formatFamily(f ebpf.ConnectionFamily) model.ConnectionFamily {
	switch f {
	case ebpf.AFINET:
		return model.ConnectionFamily_v4
	case ebpf.AFINET6:
		return model.ConnectionFamily_v6
	default:
		return -1
	}
}

func formatType(f ebpf.ConnectionType) model.ConnectionType {
	switch f {
	case ebpf.TCP:
		return model.ConnectionType_tcp
	case ebpf.UDP:
		return model.ConnectionType_udp
	default:
		return -1
	}
}

func formatDirection(d ebpf.ConnectionDirection) model.ConnectionDirection {
	switch d {
	case ebpf.INCOMING:
		return model.ConnectionDirection_incoming
	case ebpf.OUTGOING:
		return model.ConnectionDirection_outgoing
	case ebpf.LOCAL:
		return model.ConnectionDirection_local
	default:
		return model.ConnectionDirection_unspecified
	}
}

func formatIPTranslation(ct *netlink.IPTranslation) *model.IPTranslation {
	if ct == nil {
		return nil
	}

	return &model.IPTranslation{
		ReplSrcIP:   ct.ReplSrcIP,
		ReplDstIP:   ct.ReplDstIP,
		ReplSrcPort: int32(ct.ReplSrcPort),
		ReplDstPort: int32(ct.ReplDstPort),
	}
}

func batchConnections(cfg *config.AgentConfig, groupID int32, cxs []*model.Connection) []model.MessageBody {
	groupSize := groupSize(len(cxs), cfg.MaxConnsPerMessage)
	batches := make([]model.MessageBody, 0, groupSize)

	for len(cxs) > 0 {
		batchSize := min(cfg.MaxConnsPerMessage, len(cxs))
		ctrIDForPID := Process.filterCtrIDsByPIDs(connectionPIDs(cxs[:batchSize]))
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

func connectionStatsPIDs(conns []ebpf.ConnectionStats) []uint32 {
	ps := make(map[uint32]struct{}) // Map used to represent a set
	for _, c := range conns {
		ps[c.Pid] = struct{}{}
	}

	pids := make([]uint32, 0, len(ps))
	for pid := range ps {
		pids = append(pids, pid)
	}
	return pids
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
