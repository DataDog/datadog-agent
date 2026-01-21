// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package connections provides a check that collects TCP connections from the discovery module
// and sends them to the NPM backend for service dependency mapping.
package connections

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	connectionsforwarder "github.com/DataDog/datadog-agent/comp/forwarder/connectionsforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	discomodel "github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	// CheckName is the name of the check
	CheckName = "disco_connections"
)

// Config is the configuration for the disco_connections check
type Config struct {
	Enabled bool `yaml:"enabled"`
}

// Check collects TCP connections from the discovery module and sends them to the NPM backend.
type Check struct {
	core.CheckBase
	config               *Config
	tagger               tagger.Component
	connectionsForwarder connectionsforwarder.Component // may be nil
	sysProbeClient       *sysprobeclient.CheckClient
	hostname             string
}

// Factory creates a new check factory
func Factory(tagger tagger.Component, connectionsForwarder connectionsforwarder.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(tagger, connectionsForwarder)
	})
}

func newCheck(tagger tagger.Component, connectionsForwarder connectionsforwarder.Component) check.Check {
	return &Check{
		CheckBase:            core.NewCheckBase(CheckName),
		config:               &Config{},
		tagger:               tagger,
		connectionsForwarder: connectionsForwarder,
	}
}

// Parse parses the check configuration
func (c *Config) Parse(data []byte) error {
	c.Enabled = true
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	c.sysProbeClient = sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")),
	)

	c.hostname, err = hostname.Get(nil)
	if err != nil {
		log.Warnf("Error getting hostname: %v", err)
		c.hostname = "unknown"
	}

	return c.config.Parse(config)
}

// Run executes the check
func (c *Check) Run() error {
	if !c.config.Enabled {
		return nil
	}

	// 1. Fetch connections from system-probe
	resp, err := sysprobeclient.GetEndpoint[discomodel.ConnectionsResponse](c.sysProbeClient, "/connections", sysconfig.DiscoveryModule)
	if err != nil {
		return sysprobeclient.IgnoreStartupError(err)
	}

	if len(resp.Connections) == 0 {
		log.Debug("No connections found")
		return nil
	}

	log.Infof("Collected %d connections from system-probe", len(resp.Connections))

	// 2. Build NPM payload
	payload := c.buildPayload(resp.Connections)

	// 3. Send payload (or log if forwarder not available)
	return c.sendPayload(payload)
}

// getContainerIDForPID extracts the container ID for a given PID from /proc/[pid]/cgroup.
func getContainerIDForPID(pid uint32) string {
	pidStr := strconv.Itoa(int(pid))
	containerID, err := cgroups.IdentiferFromCgroupReferences(kernel.HostProc(), pidStr, "", cgroups.ContainerFilter)
	if err != nil {
		log.Debugf("Error getting container ID for PID %d: %v", pid, err)
		return ""
	}
	return containerID
}

// addrKey is used for looking up container IDs by address
type addrKey struct {
	ip   string
	port int32
}

// buildPayload builds a CollectorConnections payload from the discovered connections.
// It uses a two-pass algorithm (similar to NPM's LocalResolver) to resolve container IDs
// for both local and remote addresses when connections are on the same host.
func (c *Check) buildPayload(connections []discomodel.Connection) *model.CollectorConnections {
	npmConns := make([]*model.Connection, 0, len(connections))
	containerForPID := make(map[int32]string)
	tagsEncoder := model.NewV3TagEncoder()

	// Pass 1: Build connections and create a map of local addresses -> container ID
	// This allows us to resolve remote container IDs for intra-host connections
	laddrToContainer := make(map[addrKey]string)

	for _, conn := range connections {
		// Get container ID for this PID
		containerID := getContainerIDForPID(conn.PID)
		if containerID != "" {
			containerForPID[int32(conn.PID)] = containerID
			// Map the local address to this container ID for later raddr resolution
			laddrToContainer[addrKey{ip: conn.Laddr.IP, port: int32(conn.Laddr.Port)}] = containerID
		}

		// Get process tags from tagger
		processEntityID := types.NewEntityID(types.Process, strconv.Itoa(int(conn.PID)))
		processTags, err := c.tagger.Tag(processEntityID, types.HighCardinality)
		if err != nil {
			log.Debugf("Error getting tags for process %d: %v", conn.PID, err)
			processTags = nil
		}

		// Build connection tags
		var tagsIdx int32 = -1
		if len(processTags) > 0 {
			tagsIdx = int32(tagsEncoder.Encode(processTags))
		}

		// Convert direction
		var direction model.ConnectionDirection
		switch conn.Direction {
		case "incoming":
			direction = model.ConnectionDirection_incoming
		case "outgoing":
			direction = model.ConnectionDirection_outgoing
		default:
			direction = model.ConnectionDirection_unspecified
		}

		// Convert family
		var family model.ConnectionFamily
		if conn.Family == "v6" {
			family = model.ConnectionFamily_v6
		} else {
			family = model.ConnectionFamily_v4
		}

		npmConn := &model.Connection{
			Pid: int32(conn.PID),
			Laddr: &model.Addr{
				Ip:          conn.Laddr.IP,
				Port:        int32(conn.Laddr.Port),
				ContainerId: containerID,
			},
			Raddr: &model.Addr{
				Ip:   conn.Raddr.IP,
				Port: int32(conn.Raddr.Port),
			},
			Family:    family,
			Type:      model.ConnectionType_tcp,
			Direction: direction,
			NetNS:     conn.NetNS,
			// Required: at least one non-zero traffic counter
			LastBytesSent: 1,
			TagsIdx:       tagsIdx,
		}

		npmConns = append(npmConns, npmConn)
	}

	// Pass 2: Resolve remote container IDs for intra-host connections
	// If the remote address matches a known local address, set the remote container ID
	resolvedCount := 0
	for _, conn := range npmConns {
		if conn.Raddr.ContainerId != "" {
			continue // Already resolved
		}
		rkey := addrKey{ip: conn.Raddr.Ip, port: conn.Raddr.Port}
		if cid, ok := laddrToContainer[rkey]; ok {
			conn.Raddr.ContainerId = cid
			resolvedCount++
		}
	}
	if resolvedCount > 0 {
		log.Debugf("Resolved %d remote container IDs for intra-host connections", resolvedCount)
	}

	return &model.CollectorConnections{
		HostName:               c.hostname,
		Connections:            npmConns,
		GroupId:                1,
		GroupSize:              1, // Required: must be > 0
		ContainerForPid:        containerForPID,
		EncodedConnectionsTags: tagsEncoder.Buffer(),
	}
}

// sendPayload encodes and sends the CollectorConnections payload to the NPM backend.
// If the connections forwarder is not available, it logs the payload summary instead.
func (c *Check) sendPayload(payload *model.CollectorConnections) error {
	// Always log connections for debugging
	log.Infof("Connections payload contains %d connections:", len(payload.Connections))
	for i, conn := range payload.Connections {
		if i >= 10 {
			log.Infof("  ... and %d more connections", len(payload.Connections)-10)
			break
		}
		lctr := conn.Laddr.ContainerId
		if lctr == "" {
			lctr = "-"
		}
		rctr := conn.Raddr.ContainerId
		if rctr == "" {
			rctr = "-"
		}
		log.Infof("  [%d] %s:%d [%s] -> %s:%d [%s] (pid=%d, dir=%s)",
			i,
			conn.Laddr.Ip, conn.Laddr.Port, lctr,
			conn.Raddr.Ip, conn.Raddr.Port, rctr,
			conn.Pid, conn.Direction.String())
	}
	// Also output JSON for easy parsing
	jsonBytes, _ := json.MarshalIndent(payload, "", "  ")
	log.Debugf("Full payload:\n%s", string(jsonBytes))

	// If no forwarder, we're done
	if c.connectionsForwarder == nil {
		log.Infof("Connections forwarder not available, skipping send")
		return nil
	}

	body, err := api.EncodePayload(payload)
	if err != nil {
		return err
	}

	bytesPayload := transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&body})

	extraHeaders := make(http.Header)
	extraHeaders.Set(headers.HostHeader, c.hostname)
	extraHeaders.Set(headers.ContentTypeHeader, headers.ProtobufContentType)
	extraHeaders.Set(headers.TimestampHeader, strconv.Itoa(int(time.Now().Unix())))

	agentVersion, _ := version.Agent()
	extraHeaders.Set(headers.ProcessVersionHeader, agentVersion.GetNumber())

	_, err = c.connectionsForwarder.SubmitConnectionChecks(bytesPayload, extraHeaders)
	if err != nil {
		log.Errorf("Error sending connections payload: %v", err)
		return err
	}

	log.Infof("Successfully sent %d connections to NPM backend", len(payload.Connections))
	return nil
}
