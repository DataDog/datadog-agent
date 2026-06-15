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
	"strings"
	"syscall"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	// pingInterval is the spacing between ICMP echo requests.
	pingInterval = 100 * time.Millisecond

	// oidSysName is the device's administratively-assigned name.
	oidSysName = "1.3.6.1.2.1.1.5.0"

	// Check types; values MUST match the connectivityCheck manifest.
	checkPing = "ping"
	checkSNMP = "snmp"

	failureNone        = "none"
	failureUnreachable = "unreachable"
	failureTimeout     = "timeout"
	failureCredential  = "credential_failure"
	failureUnknown     = "unknown"
)

// PingOptions configures the ICMP reachability check.
type PingOptions struct {
	Count     int `json:"count"`
	TimeoutMs int `json:"timeoutMs"`
}

// SNMPOptions configures the SNMP check.
type SNMPOptions struct {
	Version      string `json:"version"`
	Port         int    `json:"port"`
	Community    string `json:"community,omitempty"`
	User         string `json:"user,omitempty"`
	AuthProtocol string `json:"authProtocol,omitempty"`
	AuthKey      string `json:"authKey,omitempty"`
	PrivProtocol string `json:"privProtocol,omitempty"`
	PrivKey      string `json:"privKey,omitempty"`
	ContextName  string `json:"contextName,omitempty"`
	TimeoutMs    int    `json:"timeoutMs"`
	Retries      int    `json:"retries"`
}

// ConnectivityCheckRequest is the connectivity-check input.
type ConnectivityCheckRequest struct {
	TargetAddresses []string     `json:"targetAddresses"`
	Checks          []string     `json:"checks"`
	PingOptions     *PingOptions `json:"pingOptions,omitempty"`
	SNMPOptions     *SNMPOptions `json:"snmpOptions,omitempty"`
}

// checkResult holds the fields common to every connectivity check result.
type checkResult struct {
	Success bool   `json:"success"`
	RttMs   *int64 `json:"rttMs,omitempty"`
	Error   string `json:"error,omitempty"`
}

// PingResult is the result of the ICMP reachability check.
type PingResult struct {
	checkResult
	FailureCategory string `json:"failureCategory"`
}

// SNMPResult is the result of the SNMP check.
type SNMPResult struct {
	checkResult
	FailureCategory string `json:"failureCategory"`
	SysName         string `json:"sysName,omitempty"`
}

// DeviceResult holds the connectivity results for a single resolved device address.
type DeviceResult struct {
	IPAddress  string      `json:"ipAddress"`
	PingResult *PingResult `json:"pingResult,omitempty"`
	SNMPResult *SNMPResult `json:"snmpResult,omitempty"`
}

// ConnectivityCheckResult is the connectivity-check output.
type ConnectivityCheckResult struct {
	Devices []DeviceResult `json:"devices"`
}

// ConnectivityCheckHandler implements the connectivityCheck PAR action.
type ConnectivityCheckHandler struct{}

// NewConnectivityCheckHandler creates a new ConnectivityCheckHandler.
func NewConnectivityCheckHandler() *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{}
}

// Run parses the action inputs and runs the connectivity check.
func (h *ConnectivityCheckHandler) Run(
	ctx context.Context,
	task *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	req, err := types.ExtractInputs[ConnectivityCheckRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}

	res, err := runChecks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("connectivityCheck: %w", err)
	}

	return res, nil
}

// runChecks runs each requested check against every resolved target address.
func runChecks(ctx context.Context, req ConnectivityCheckRequest) (ConnectivityCheckResult, error) {
	if len(req.TargetAddresses) == 0 {
		return ConnectivityCheckResult{}, errors.New("at least one target address is required")
	}
	if len(req.Checks) == 0 {
		return ConnectivityCheckResult{}, errors.New("at least one check is required")
	}

	hosts, err := expandTargets(req.TargetAddresses)
	if err != nil {
		return ConnectivityCheckResult{}, err
	}

	devices := make([]DeviceResult, 0, len(hosts))
	for _, host := range hosts {
		if ctx.Err() != nil {
			break
		}
		dr := DeviceResult{IPAddress: host}
		for _, c := range req.Checks {
			switch c {
			case checkPing:
				dr.PingResult = runPing(host, req.PingOptions)
			case checkSNMP:
				dr.SNMPResult = runSNMP(ctx, host, req.SNMPOptions)
			}
		}
		devices = append(devices, dr)
	}
	return ConnectivityCheckResult{Devices: devices}, nil
}

func runPing(host string, opts *PingOptions) *PingResult {
	if opts == nil {
		return &PingResult{checkResult: checkResult{Success: false, Error: "ping options are required"}, FailureCategory: failureUnknown}
	}

	p, err := pinger.New(pinger.Config{
		UseRawSocket: false,
		Count:        opts.Count,
		Interval:     pingInterval,
		Timeout:      time.Duration(opts.TimeoutMs) * time.Millisecond,
	})
	if err != nil {
		return &PingResult{checkResult: checkResult{Success: false, Error: err.Error()}, FailureCategory: failureUnknown}
	}

	result, err := p.Ping(host)
	if err != nil {
		return &PingResult{checkResult: checkResult{Success: false, Error: err.Error()}, FailureCategory: failureUnreachable}
	}
	if result == nil || !result.CanConnect {
		return &PingResult{checkResult: checkResult{Success: false}, FailureCategory: failureUnreachable}
	}

	rtt := result.AvgRtt.Milliseconds()
	return &PingResult{checkResult: checkResult{Success: true, RttMs: &rtt}, FailureCategory: failureNone}
}

// runSNMP GETs sysName, measures the request round-trip, and classifies failures.
func runSNMP(ctx context.Context, host string, opts *SNMPOptions) *SNMPResult {
	if opts == nil || opts.Version == "" {
		return &SNMPResult{checkResult: checkResult{Success: false, Error: "snmp options (version) are required when 'snmp' is requested"}, FailureCategory: failureUnknown}
	}

	client, err := buildSNMPClient(ctx, host, opts)
	if err != nil {
		return &SNMPResult{checkResult: checkResult{Success: false, Error: err.Error()}, FailureCategory: failureUnknown}
	}

	if err := client.Connect(); err != nil {
		category, msg := classifySNMPError(err)
		return &SNMPResult{checkResult: checkResult{Success: false, Error: msg}, FailureCategory: category}
	}
	defer func() { _ = client.Conn.Close() }()

	start := time.Now()
	packet, err := client.Get([]string{oidSysName})
	if err != nil {
		category, msg := classifySNMPError(err)
		return &SNMPResult{checkResult: checkResult{Success: false, Error: msg}, FailureCategory: category}
	}
	rtt := time.Since(start).Milliseconds()

	res := &SNMPResult{checkResult: checkResult{Success: true, RttMs: &rtt}, FailureCategory: failureNone}
	for _, pdu := range packet.Variables {
		value, convErr := gosnmplib.GetValueFromPDU(pdu)
		if convErr != nil {
			continue
		}
		str, convErr := gosnmplib.StandardTypeToString(value)
		if convErr != nil {
			continue
		}
		if strings.TrimLeft(pdu.Name, ".") == oidSysName {
			res.SysName = str
		}
	}
	return res
}

// buildSNMPClient builds a gosnmp client from opts.
func buildSNMPClient(ctx context.Context, host string, opts *SNMPOptions) (*gosnmp.GoSNMP, error) {
	version, err := snmpVersion(opts.Version)
	if err != nil {
		return nil, err
	}

	client := &gosnmp.GoSNMP{
		Context:     ctx,
		Target:      host,
		Port:        uint16(opts.Port),
		Transport:   "udp",
		Version:     version,
		Timeout:     time.Duration(opts.TimeoutMs) * time.Millisecond,
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
		client.Community = opts.Community
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

// v3MsgFlags derives the v3 security level from the supplied credentials.
func v3MsgFlags(opts *SNMPOptions) gosnmp.SnmpV3MsgFlags {
	switch {
	case opts.PrivKey != "":
		return gosnmp.AuthPriv
	case opts.AuthKey != "":
		return gosnmp.AuthNoPriv
	default:
		return gosnmp.NoAuthNoPriv
	}
}

// classifySNMPError maps a gosnmp connect/get error to a failureCategory. The gosnmp v3 USM
// sentinels and the %w-wrapped socket errnos are matched by identity; only the timeout, which
// gosnmp re-creates as a bare string, needs a substring match.
func classifySNMPError(err error) (category, message string) {
	message = err.Error()
	switch {
	case errors.Is(err, gosnmp.ErrWrongDigest),
		errors.Is(err, gosnmp.ErrDecryption),
		errors.Is(err, gosnmp.ErrUnknownUsername),
		errors.Is(err, gosnmp.ErrUnknownSecurityLevel),
		errors.Is(err, gosnmp.ErrUnknownEngineID):
		return failureCredential, message
	case errors.Is(err, context.DeadlineExceeded):
		return failureTimeout, message
	case errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH):
		return failureUnreachable, message
	}
	if strings.Contains(strings.ToLower(message), "request timeout") {
		return failureTimeout, message
	}
	return failureUnknown, message
}

// expandTargets resolves IPs and CIDR ranges into a host list.
func expandTargets(targets []string) ([]string, error) {
	var hosts []string
	for _, t := range targets {
		if prefix, err := netip.ParsePrefix(t); err == nil {
			for addr := prefix.Masked().Addr(); prefix.Contains(addr); addr = addr.Next() {
				hosts = append(hosts, addr.String())
				if !addr.Next().IsValid() {
					break
				}
			}
			continue
		}
		if addr, err := netip.ParseAddr(t); err == nil {
			hosts = append(hosts, addr.String())
			continue
		}
		return nil, fmt.Errorf("invalid target %q (expected an IP address or CIDR range)", t)
	}
	return hosts, nil
}
