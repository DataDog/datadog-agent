// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package connectivity runs on-host connectivity checks (ICMP, SNMP) against network
// devices and classifies failures. It backs both the connectivityCheck Private Action
// and the `datadog-agent snmp connectivity` CLI, so the two share identical logic.
package connectivity

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
)

const (
	defaultPingCount   = 3
	defaultPingTimeout = 3 * time.Second
	pingInterval       = 100 * time.Millisecond

	// CheckPing and CheckSNMP are the supported check types. These string values MUST match
	// the com.datadoghq.remoteaction.networkdevices.connectivityCheck manifest unions exactly.
	CheckPing = "ping"
	CheckSNMP = "snmp"

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

// SnmpOptions configures the SNMP check. Credentials are sensitive: they are never returned
// in outputs.
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

// Request is the connectivity-check input. The JSON tags match the
// com.datadoghq.remoteaction.networkdevices.connectivityCheck manifest.
type Request struct {
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

// Result is the connectivity-check output.
type Result struct {
	Devices []DeviceResult `json:"devices"`
}

// Run expands the request targets into host addresses and runs each requested check against
// every device, classifying any failures. It runs entirely on the local host (no backend).
func Run(ctx context.Context, req Request) (Result, error) {
	if len(req.Targets) == 0 {
		return Result{}, errors.New("at least one target is required")
	}
	if len(req.Checks) == 0 {
		return Result{}, errors.New("at least one check is required")
	}

	hosts, err := expandTargets(req.Targets)
	if err != nil {
		return Result{}, err
	}

	devices := make([]DeviceResult, 0, len(hosts))
	for _, host := range hosts {
		if ctx.Err() != nil {
			break
		}
		dr := DeviceResult{IPAddress: host, Succeeded: true}
		for _, c := range req.Checks {
			var res CheckResult
			switch c {
			case CheckPing:
				res = runPing(host, req.Ping)
			case CheckSNMP:
				res = runSNMP(ctx, host, req.Snmp)
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
	return Result{Devices: devices}, nil
}

// runPing performs the ICMP reachability check by reusing the Agent's pinger.
func runPing(host string, opts *PingOptions) CheckResult {
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
		UseRawSocket: false,
		Count:        count,
		Interval:     pingInterval,
		Timeout:      timeout,
	})
	if err != nil {
		return CheckResult{Type: CheckPing, Success: false, FailureCategory: failureUnknown, Error: err.Error()}
	}

	result, err := p.Ping(host)
	if err != nil {
		return CheckResult{Type: CheckPing, Success: false, FailureCategory: failureUnreachable, Error: err.Error()}
	}
	if result == nil || !result.CanConnect {
		// No ICMP echo replies were received within the timeout window.
		return CheckResult{Type: CheckPing, Success: false, FailureCategory: failureUnreachable}
	}

	rtt := result.AvgRtt.Milliseconds()
	return CheckResult{Type: CheckPing, Success: true, FailureCategory: failureNone, RttMs: &rtt}
}

// runSNMP performs the SNMP reachability + identity check (sysObjectID / sysDescr) and
// classifies credential / version failures.
//
// TODO(NDM): not yet implemented (deferred with the credentials work). Intended approach:
//   - build a *snmpparse.SNMPConfig from opts → snmpparse.NewSNMP(cfg, logger) → Connect()
//   - GET sysObjectID (1.3.6.1.2.1.1.2.0) and sysDescr (1.3.6.1.2.1.1.1.0)
//   - classify: no UDP/161 response -> unreachable/timeout; auth/decrypt error ->
//     credential_failure; version/report PDU -> version_mismatch
func runSNMP(_ context.Context, _ string, opts *SnmpOptions) CheckResult {
	if opts == nil || opts.Version == "" {
		return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: failureUnknown, Error: "snmp options (version) are required when 'snmp' is requested"}
	}
	return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: failureUnknown, Error: "snmp check not yet implemented"}
}

// expandTargets resolves the input targets (individual IPs and CIDR ranges) into a
// de-duplicated list of host addresses. The caller (backend) is responsible for bounding
// the size of the targets it sends.
func expandTargets(targets []string) ([]string, error) {
	var hosts []string
	seen := make(map[string]struct{})
	add := func(s string) {
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		hosts = append(hosts, s)
	}

	for _, t := range targets {
		// CIDR range -> sweep every address in the prefix.
		if prefix, err := netip.ParsePrefix(t); err == nil {
			for addr := prefix.Masked().Addr(); prefix.Contains(addr); addr = addr.Next() {
				add(addr.String())
				if !addr.Next().IsValid() {
					break
				}
			}
			continue
		}
		// Single IP address.
		if addr, err := netip.ParseAddr(t); err == nil {
			add(addr.String())
			continue
		}
		return nil, fmt.Errorf("invalid target %q (expected an IP address or CIDR range)", t)
	}
	return hosts, nil
}
