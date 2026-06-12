// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

const (
	// maxExpandedTargets bounds CIDR expansion so a single task stays within its timeout
	// and within the Action Platform's per-action limits.
	maxExpandedTargets = 1024

	defaultPingCount   = 3
	defaultPingTimeout = 3 * time.Second
	pingInterval       = 100 * time.Millisecond

	// TODO(NDM): the PAR action runs inside the Agent process; confirm it has the
	// privileges the pinger needs. Raw sockets require CAP_NET_RAW; with UseRawSocket=false
	// the Linux pinger routes through system-probe. This default may need to come from
	// agent config (e.g. network_devices.connectivity_check.use_raw_sockets).
	useRawSocket = true
)

// Check types and failure categories. These string values MUST match the
// com.datadoghq.remoteaction.networkdevices.connectivityCheck manifest unions exactly.
const (
	checkPing = "ping"
	checkSNMP = "snmp"

	failureNone            = "none"
	failureUnreachable     = "unreachable"
	failureTimeout         = "timeout"
	failureCredential      = "credential_failure"
	failureVersionMismatch = "version_mismatch"
	failureUnknown         = "unknown"
)

// PingOptions configures the ICMP reachability check.
type PingOptions struct {
	Count     int `json:"count,omitempty"`
	TimeoutMs int `json:"timeoutMs,omitempty"`
}

// SnmpOptions configures the SNMP check. Credentials are sensitive: they are scrubbed by
// the Agent and never returned in outputs.
type SnmpOptions struct {
	Version      string `json:"version"`
	Port         int    `json:"port,omitempty"`
	Community    string `json:"community,omitempty"`
	User         string `json:"user,omitempty"`
	AuthProtocol string `json:"authProtocol,omitempty"`
	AuthKey      string `json:"authKey,omitempty"`
	PrivProtocol string `json:"privProtocol,omitempty"`
	PrivKey      string `json:"privKey,omitempty"`
	ContextName  string `json:"contextName,omitempty"`
	TimeoutMs    int    `json:"timeoutMs,omitempty"`
	Retries      int    `json:"retries,omitempty"`
}

// ConnectivityCheckInputs is the input contract for the connectivityCheck action.
type ConnectivityCheckInputs struct {
	Targets []string     `json:"targets"`
	Checks  []string     `json:"checks"`
	Ping    *PingOptions `json:"ping,omitempty"`
	Snmp    *SnmpOptions `json:"snmp,omitempty"`
}

// CheckResult is the result of a single connectivity check against a device.
type CheckResult struct {
	Type            string `json:"type"`
	Success         bool   `json:"success"`
	FailureCategory string `json:"failureCategory"`
	RttMs           *int64 `json:"rttMs,omitempty"`
	SysObjectID     string `json:"sysObjectId,omitempty"`
	SysDescr        string `json:"sysDescr,omitempty"`
	Error           string `json:"error,omitempty"`
}

// DeviceResult holds the connectivity results for a single resolved device address.
type DeviceResult struct {
	IPAddress string        `json:"ipAddress"`
	Succeeded bool          `json:"succeeded"`
	Results   []CheckResult `json:"results"`
}

// ConnectivityCheckOutputs is the output contract for the connectivityCheck action.
type ConnectivityCheckOutputs struct {
	Devices []DeviceResult `json:"devices"`
}

// ConnectivityCheckHandler implements the connectivityCheck action.
type ConnectivityCheckHandler struct{}

// NewConnectivityCheckHandler creates a new ConnectivityCheckHandler.
func NewConnectivityCheckHandler() *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{}
}

// Run expands the targets to host addresses and runs each requested check against every
// device, classifying any failures. It is a no-credential, read-only action.
func (h *ConnectivityCheckHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[ConnectivityCheckInputs](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}
	if len(inputs.Targets) == 0 {
		return nil, errors.New("connectivityCheck: at least one target is required")
	}
	if len(inputs.Checks) == 0 {
		return nil, errors.New("connectivityCheck: at least one check is required")
	}

	hosts, err := expandTargets(inputs.Targets)
	if err != nil {
		return nil, fmt.Errorf("connectivityCheck: %w", err)
	}

	devices := make([]DeviceResult, 0, len(hosts))
	for _, host := range hosts {
		if ctx.Err() != nil {
			break
		}
		dr := DeviceResult{IPAddress: host, Succeeded: true}
		for _, c := range inputs.Checks {
			var res CheckResult
			switch c {
			case checkPing:
				res = h.runPing(host, inputs.Ping)
			case checkSNMP:
				res = h.runSNMP(ctx, host, inputs.Snmp)
			default:
				res = CheckResult{Type: c, Success: false, FailureCategory: failureUnknown, Error: fmt.Sprintf("unsupported check %q", c)}
			}
			if !res.Success {
				dr.Succeeded = false
			}
			dr.Results = append(dr.Results, res)
		}
		devices = append(devices, dr)
	}

	return ConnectivityCheckOutputs{Devices: devices}, nil
}

// runPing performs the ICMP reachability check by reusing the Agent's pinger.
func (h *ConnectivityCheckHandler) runPing(host string, opts *PingOptions) CheckResult {
	count := defaultPingCount
	timeout := defaultPingTimeout
	if opts != nil {
		if opts.Count > 0 {
			count = opts.Count
		}
		if opts.TimeoutMs > 0 {
			timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
		}
	}

	p, err := pinger.New(pinger.Config{
		UseRawSocket: useRawSocket,
		Count:        count,
		Interval:     pingInterval,
		Timeout:      timeout,
	})
	if err != nil {
		return CheckResult{Type: checkPing, Success: false, FailureCategory: failureUnknown, Error: err.Error()}
	}

	result, err := p.Ping(host)
	if err != nil {
		return CheckResult{Type: checkPing, Success: false, FailureCategory: failureUnreachable, Error: err.Error()}
	}
	if result == nil || !result.CanConnect {
		// No ICMP echo replies were received within the timeout window.
		return CheckResult{Type: checkPing, Success: false, FailureCategory: failureUnreachable}
	}

	rtt := result.AvgRtt.Milliseconds()
	return CheckResult{Type: checkPing, Success: true, FailureCategory: failureNone, RttMs: &rtt}
}

// runSNMP performs the SNMP reachability + identity check (sysObjectID / sysDescr) and
// classifies credential / version failures.
//
// TODO(NDM): not yet implemented (deferred with the credentials work). Intended approach
// — no new connection type; credentials come inline in SnmpOptions, are scrubbed by the
// Agent, and are never returned in outputs:
//   - build a *snmpparse.SNMPConfig from opts (Version, CommunityString, Username,
//     AuthProtocol/AuthKey, PrivProtocol/PrivKey, ContextName, Port, Timeout, Retries)
//   - conn, err := snmpparse.NewSNMP(cfg, logger); conn.Connect()
//   - GET sysObjectID (1.3.6.1.2.1.1.2.0) and sysDescr (1.3.6.1.2.1.1.1.0)
//   - classify: no UDP/161 response -> unreachable / timeout; auth or decrypt error ->
//     credential_failure; version/report PDU -> version_mismatch
//
// Implementing this requires threading a log.Component into the bundle, since
// snmpparse.NewSNMP needs one (the bundle constructor / registry would pass it).
func (h *ConnectivityCheckHandler) runSNMP(_ context.Context, _ string, opts *SnmpOptions) CheckResult {
	if opts == nil || opts.Version == "" {
		return CheckResult{Type: checkSNMP, Success: false, FailureCategory: failureUnknown, Error: "snmp options (version) are required when 'snmp' is requested"}
	}
	return CheckResult{Type: checkSNMP, Success: false, FailureCategory: failureUnknown, Error: "snmp check not yet implemented"}
}

// expandTargets resolves the input targets (individual IPs and CIDR ranges) into a bounded,
// de-duplicated list of host addresses to check.
func expandTargets(targets []string) ([]string, error) {
	var hosts []string
	seen := make(map[string]struct{})
	add := func(s string) bool {
		if _, ok := seen[s]; ok {
			return true
		}
		if len(hosts) >= maxExpandedTargets {
			return false
		}
		seen[s] = struct{}{}
		hosts = append(hosts, s)
		return true
	}

	for _, t := range targets {
		// CIDR range -> sweep every address in the prefix.
		if prefix, err := netip.ParsePrefix(t); err == nil {
			for addr := prefix.Masked().Addr(); prefix.Contains(addr); addr = addr.Next() {
				if !add(addr.String()) {
					return nil, fmt.Errorf("targets expand to more than %d addresses; use smaller ranges", maxExpandedTargets)
				}
				if !addr.Next().IsValid() {
					break
				}
			}
			continue
		}
		// Single IP address.
		if addr, err := netip.ParseAddr(t); err == nil {
			if !add(addr.String()) {
				return nil, fmt.Errorf("targets expand to more than %d addresses; use smaller ranges", maxExpandedTargets)
			}
			continue
		}
		return nil, fmt.Errorf("invalid target %q (expected an IP address or CIDR range)", t)
	}
	return hosts, nil
}
