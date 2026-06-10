// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ndmconnectivitycheckimpl implements the ndmconnectivitycheck component interface.
package ndmconnectivitycheckimpl

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	ndmconnectivitycheck "github.com/DataDog/datadog-agent/comp/ndmconnectivitycheck/def"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
)

const defaultNamespace = "default"

// Requires defines the dependencies for the ndmconnectivitycheck component.
type Requires struct {
	compdef.In
	Logger        log.Component
	Config        config.Component
	EventPlatform eventplatform.Component
}

// Provides defines the output of the ndmconnectivitycheck component.
type Provides struct {
	Comp       ndmconnectivitycheck.Component
	RCListener rcclienttypes.TaskListenerProvider
}

// NewComponent creates a new ndmconnectivitycheck component.
func NewComponent(reqs Requires) (Provides, error) {
	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		return Provides{}, errors.New("event platform forwarder not initialized")
	}

	checker := &connectivityCheckerImpl{
		log:         reqs.Logger,
		config:      reqs.Config,
		epforwarder: forwarder,
		pingCfg: pinger.Config{
			// UDP/raw socket selection follows the same model as the SNMP check's ping:
			// raw sockets require CAP_NET_RAW; otherwise the request is routed through
			// system-probe. Defaulting to config so deployments can opt in/out.
			UseRawSocket: reqs.Config.GetBool("network_devices.connectivity_check.use_raw_sockets"),
			Count:        3,
			Interval:     20 * time.Millisecond,
			Timeout:      3 * time.Second,
		},
	}

	return Provides{
		Comp:       checker,
		RCListener: rcclienttypes.NewTaskListener(checker.handleAgentTask),
	}, nil
}

type connectivityCheckerImpl struct {
	log         log.Component
	config      config.Component
	epforwarder eventplatform.Forwarder
	pingCfg     pinger.Config
}

// handleAgentTask is the RCAgentTaskListener entrypoint. It only handles the
// connectivity-check task type and ignores everything else.
func (c *connectivityCheckerImpl) handleAgentTask(taskType rcclienttypes.TaskType, task rcclienttypes.AgentTaskConfig) (bool, error) {
	if taskType != rcclienttypes.TaskConnectivityCheck {
		return false, nil
	}

	ip, ok := task.Config.TaskArgs["ip_address"]
	if !ok || ip == "" {
		return true, errors.New("connectivity check task is missing required 'ip_address' arg")
	}

	c.CheckConnectivity(ndmconnectivitycheck.Request{
		DeviceIPs: []string{ip},
		Namespace: task.Config.TaskArgs["namespace"],
	})
	return true, nil
}

// CheckConnectivity pings each IP and reports the per-device reachability.
func (c *connectivityCheckerImpl) CheckConnectivity(req ndmconnectivitycheck.Request) {
	namespace := req.Namespace
	if namespace == "" {
		if namespace = c.config.GetString("network_devices.namespace"); namespace == "" {
			namespace = defaultNamespace
		}
	}

	for _, ip := range req.DeviceIPs {
		status := c.pingOne(ip)
		if err := c.sendPingResult(namespace, ip, status); err != nil {
			c.log.Errorf("Unable to report connectivity result for %s: %v", ip, err)
		}
	}
}

// pingOne returns the reachability status for a single IP. A failed ping (or an
// error constructing/running the pinger) is reported as Unreachable rather than
// failing the whole task, so a UI validation step sees a definitive per-IP answer.
func (c *connectivityCheckerImpl) pingOne(ip string) metadata.DeviceStatus {
	p, err := pinger.New(c.pingCfg)
	if err != nil {
		c.log.Warnf("Could not create pinger for %s: %v", ip, err)
		return metadata.DeviceStatusUnreachable
	}

	result, err := p.Ping(ip)
	if err != nil {
		c.log.Debugf("Ping to %s failed: %v", ip, err)
		return metadata.DeviceStatusUnreachable
	}
	if result.CanConnect {
		return metadata.DeviceStatusReachable
	}
	return metadata.DeviceStatusUnreachable
}

// sendPingResult emits a network-devices-metadata payload carrying the ping result
// for a single device IP, mirroring how snmpscan reports scan status.
//
// NOTE (design, to coordinate with the NDM backend): emitting a DeviceMetadata may
// cause the EVP consumer to add the IP to the device inventory. For a pre-enrollment
// connectivity check the result should likely be treated as ephemeral (validation
// only) and NOT auto-enroll the device. The exact backend semantics are a follow-up.
func (c *connectivityCheckerImpl) sendPingResult(namespace, ip string, status metadata.DeviceStatus) error {
	payload := metadata.NetworkDevicesMetadata{
		Namespace:        namespace,
		Integration:      "snmp",
		CollectTimestamp: time.Now().Unix(),
		Devices: []metadata.DeviceMetadata{
			{
				ID:         namespace + ":" + ip,
				IPAddress:  ip,
				PingStatus: status,
			},
		},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	m := message.NewMessage(payloadBytes, nil, "", 0)
	return c.epforwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata)
}
