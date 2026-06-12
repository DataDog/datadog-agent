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
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	defaultPingCount   = 3
	defaultPingTimeout = 3 * time.Second
	pingInterval       = 100 * time.Millisecond

	// SNMP identity OIDs (scalar .0 instances). A successful GET of these proves SNMP
	// reachability plus valid credentials, and identifies the device.
	oidSysDescr    = "1.3.6.1.2.1.1.1.0"
	oidSysObjectID = "1.3.6.1.2.1.1.2.0"

	defaultSNMPTimeout   = 3 * time.Second
	defaultSNMPPort      = 161
	defaultSNMPCommunity = "public"

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

// runSNMP performs the SNMP reachability + identity check by GET-ing sysObjectID and sysDescr,
// and classifies credential / version / reachability failures. Credentials are taken from opts
// in the clear (first iteration); they are used only to build the local SNMP client and are
// never echoed back in the result.
func runSNMP(ctx context.Context, host string, opts *SnmpOptions) CheckResult {
	if opts == nil || opts.Version == "" {
		return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: failureUnknown, Error: "snmp options (version) are required when 'snmp' is requested"}
	}

	client, err := buildSNMPClient(ctx, host, opts)
	if err != nil {
		// Bad version / auth / priv inputs: a configuration problem, not a device response.
		return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: failureUnknown, Error: err.Error()}
	}

	if err := client.Connect(); err != nil {
		category, msg := classifySNMPError(err)
		return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: category, Error: msg}
	}
	defer func() { _ = client.Conn.Close() }()

	packet, err := client.Get([]string{oidSysObjectID, oidSysDescr})
	if err != nil {
		category, msg := classifySNMPError(err)
		return CheckResult{Type: CheckSNMP, Success: false, FailureCategory: category, Error: msg}
	}

	// The device answered, so it is reachable and the credentials were accepted. Decode the
	// identity OIDs best-effort (a missing scalar yields an unconvertible PDU we simply skip).
	res := CheckResult{Type: CheckSNMP, Success: true, FailureCategory: failureNone}
	for _, pdu := range packet.Variables {
		value, convErr := gosnmplib.GetValueFromPDU(pdu)
		if convErr != nil {
			continue
		}
		str, convErr := gosnmplib.StandardTypeToString(value)
		if convErr != nil {
			continue
		}
		switch strings.TrimLeft(pdu.Name, ".") {
		case oidSysObjectID:
			res.SysObjectID = str
		case oidSysDescr:
			res.SysDescr = str
		}
	}
	return res
}

// buildSNMPClient builds a gosnmp client from the (clear-text) options. It is intentionally
// dependency-free (no log.Component), so the same logic backs both the PAR action and the CLI.
func buildSNMPClient(ctx context.Context, host string, opts *SnmpOptions) (*gosnmp.GoSNMP, error) {
	version, err := snmpVersion(opts.Version)
	if err != nil {
		return nil, err
	}

	port := uint16(defaultSNMPPort)
	if opts.Port > 0 {
		port = uint16(opts.Port)
	}
	timeout := defaultSNMPTimeout
	if opts.TimeoutMs > 0 {
		timeout = time.Duration(opts.TimeoutMs) * time.Millisecond
	}

	client := &gosnmp.GoSNMP{
		Context:     ctx,
		Target:      host,
		Port:        port,
		Transport:   "udp",
		Version:     version,
		Timeout:     timeout,
		Retries:     opts.Retries,
		ContextName: opts.ContextName,
	}

	if version == gosnmp.Version3 {
		authProtocol, err := gosnmplib.GetAuthProtocol(opts.AuthProtocol)
		if err != nil {
			return nil, err
		}
		privProtocol, err := gosnmplib.GetPrivProtocol(opts.PrivProtocol)
		if err != nil {
			return nil, err
		}
		client.SecurityModel = gosnmp.UserSecurityModel
		client.MsgFlags = v3MsgFlags(opts)
		client.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 opts.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: opts.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        opts.PrivKey,
		}
	} else {
		community := opts.Community
		if community == "" {
			community = defaultSNMPCommunity
		}
		client.Community = community
	}
	return client, nil
}

// snmpVersion maps the manifest version string ("1" | "2c" | "3") to a gosnmp version.
func snmpVersion(version string) (gosnmp.SnmpVersion, error) {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "1":
		return gosnmp.Version1, nil
	case "2", "2c":
		return gosnmp.Version2c, nil
	case "3":
		return gosnmp.Version3, nil
	default:
		return 0, fmt.Errorf("unsupported snmp version %q (expected 1, 2c, or 3)", version)
	}
}

// v3MsgFlags derives the v3 security level from which credentials were supplied, matching the
// behavior of snmpparse.NewSNMP: priv key -> AuthPriv, auth key only -> AuthNoPriv, else
// NoAuthNoPriv.
func v3MsgFlags(opts *SnmpOptions) gosnmp.SnmpV3MsgFlags {
	switch {
	case opts.PrivKey != "":
		return gosnmp.AuthPriv
	case opts.AuthKey != "":
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

// classifySNMPError maps a gosnmp connect/get error to a manifest failureCategory. The match is
// heuristic (gosnmp surfaces these conditions only as error strings); the original message is
// returned alongside so callers retain the detail.
func classifySNMPError(err error) (category, message string) {
	message = err.Error()
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "request timeout"):
		return failureTimeout, message
	case containsAny(lower,
		"decryption", "wrong digest", "unknown username", "not authentic",
		"authentication parameters are not configured", "privacy parameters are not configured",
		"unknown security level", "passphrase is required", "password is empty", "unknown engine id"):
		return failureCredential, message
	case containsAny(lower, "connection refused", "no route to host", "network is unreachable", "no such host"):
		return failureUnreachable, message
	case strings.Contains(lower, "version"):
		return failureVersionMismatch, message
	default:
		return failureUnknown, message
	}
}

// containsAny reports whether s contains any of the given substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
