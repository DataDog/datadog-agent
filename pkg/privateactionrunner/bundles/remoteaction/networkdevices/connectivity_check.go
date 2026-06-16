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

	oidSysName = "1.3.6.1.2.1.1.5.0"

	checkPing = "ping"
	checkSNMP = "snmp"

	failureNone        = "none"
	failureUnreachable = "unreachable"
	failureTimeout     = "timeout"
	failureCredential  = "credential_failure"
	failureUnknown     = "unknown"
)

type PingOptions struct {
	Count     int `json:"count"`
	TimeoutMs int `json:"timeoutMs"`
}

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

type ConnectivityCheckRequest struct {
	TargetAddresses []string     `json:"targetAddresses"`
	Checks          []string     `json:"checks"`
	PingOptions     *PingOptions `json:"pingOptions,omitempty"`
	SNMPOptions     *SNMPOptions `json:"snmpOptions,omitempty"`
}

type CheckResult struct {
	Success bool   `json:"success"`
	RttMs   *int64 `json:"rttMs,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PingResult struct {
	CheckResult
	FailureCategory string `json:"failureCategory"`
}

type SNMPResult struct {
	CheckResult
	FailureCategory string `json:"failureCategory"`
	SysName         string `json:"sysName,omitempty"`
}

type DeviceResult struct {
	IPAddress  string      `json:"ipAddress"`
	PingResult *PingResult `json:"pingResult,omitempty"`
	SNMPResult *SNMPResult `json:"snmpResult,omitempty"`
}

type ConnectivityCheckResult struct {
	Devices []DeviceResult `json:"devices"`
}

type ConnectivityCheckHandler struct{}

func NewConnectivityCheckHandler() *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{}
}

func (h *ConnectivityCheckHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	req, err := types.ExtractInputs[ConnectivityCheckRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}

	res, err := runChecks(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to run connectivity checks: %w", err)
	}

	return res, nil
}

func runChecks(ctx context.Context, req ConnectivityCheckRequest) (ConnectivityCheckResult, error) {
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

func expandTargets(targetAddresses []string) ([]string, error) {
	var hosts []string

	for _, a := range targetAddresses {
		pre, err := netip.ParsePrefix(a)
		if err == nil {
			for addr := pre.Masked().Addr(); pre.Contains(addr); addr = addr.Next() {
				if !addr.IsValid() {
					break
				}

				hosts = append(hosts, addr.String())
			}

			continue
		}

		addr, err := netip.ParseAddr(a)
		if err == nil {
			hosts = append(hosts, addr.String())

			continue
		}

		return nil, fmt.Errorf("invalid target address: %s", a)
	}

	return hosts, nil
}

func runPing(host string, opts *PingOptions) *PingResult {
	if opts == nil {
		return &PingResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Ping options are required for host '%s'", host)},
			FailureCategory: failureUnknown,
		}
	}

	p, err := pinger.New(pinger.Config{
		Count:        opts.Count,
		Timeout:      time.Duration(opts.TimeoutMs) * time.Millisecond,
		Interval:     pingInterval,
		UseRawSocket: false,
	})
	if err != nil {
		return &PingResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to create pinger for host '%s': %s", host, err.Error())},
			FailureCategory: failureUnknown,
		}
	}

	res, err := p.Ping(host)
	if err != nil {
		return &PingResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to reach host '%s': %s", host, err.Error())},
			FailureCategory: failureUnreachable,
		}
	}
	if res == nil || !res.CanConnect {
		return &PingResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to connect to host '%s'", host)},
			FailureCategory: failureUnreachable,
		}
	}

	rtt := res.AvgRtt.Milliseconds()
	return &PingResult{
		CheckResult:     CheckResult{Success: true, RttMs: &rtt},
		FailureCategory: failureNone,
	}
}

func runSNMP(ctx context.Context, host string, opts *SNMPOptions) *SNMPResult {
	if opts == nil {
		return &SNMPResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("SNMP options are required for host '%s'", host)},
			FailureCategory: failureUnknown,
		}
	}

	client, err := buildSNMPClient(ctx, host, opts)
	if err != nil {
		return &SNMPResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to create SNMP client for host '%s': %s", host, err.Error())},
			FailureCategory: failureUnknown,
		}
	}

	err = client.Connect()
	if err != nil {
		return &SNMPResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to connect to SNMP host '%s': %s", host, err.Error())},
			FailureCategory: classifySNMPError(err),
		}
	}
	defer func() { _ = client.Conn.Close() }()

	startTime := time.Now()
	packet, err := client.Get([]string{oidSysName})
	if err != nil {
		return &SNMPResult{
			CheckResult:     CheckResult{Error: fmt.Sprintf("Failed to fetch device name for host '%s': %s", host, err.Error())},
			FailureCategory: classifySNMPError(err),
		}
	}
	rtt := time.Since(startTime).Milliseconds()

	res := &SNMPResult{
		CheckResult:     CheckResult{Success: true, RttMs: &rtt},
		FailureCategory: failureNone,
	}
	for _, pdu := range packet.Variables {
		v, convErr := gosnmplib.GetValueFromPDU(pdu)
		if convErr != nil {
			continue
		}

		strValue, convErr := gosnmplib.StandardTypeToString(v)
		if convErr != nil {
			continue
		}

		if strings.TrimLeft(pdu.Name, ".") == oidSysName {
			res.SysName = strValue
		}
	}

	return res
}

func buildSNMPClient(ctx context.Context, host string, opts *SNMPOptions) (*gosnmp.GoSNMP, error) {
	var ver gosnmp.SnmpVersion
	switch opts.Version {
	case "1":
		ver = gosnmp.Version1
	case "2c":
		ver = gosnmp.Version2c
	case "3":
		ver = gosnmp.Version3
	default:
		return nil, fmt.Errorf("unknown SNMP version '%s' (expected 1, 2c, or 3)", opts.Version)
	}

	c := &gosnmp.GoSNMP{
		Context:   ctx,
		Target:    host,
		Port:      uint16(opts.Port),
		Transport: "udp",
		Version:   ver,
		Timeout:   time.Duration(opts.TimeoutMs) * time.Millisecond,
		Retries:   opts.Retries,
	}

	if ver == gosnmp.Version1 || ver == gosnmp.Version2c {
		c.Community = opts.Community
	}
	if ver == gosnmp.Version3 {
		authProtocol, err := gosnmplib.GetAuthProtocol(opts.AuthProtocol)
		if err != nil {
			return nil, err
		}

		privProtocol, err := gosnmplib.GetPrivProtocol(opts.PrivProtocol)
		if err != nil {
			return nil, err
		}

		switch {
		case opts.PrivKey != "":
			c.MsgFlags = gosnmp.AuthPriv
		case opts.AuthKey != "":
			c.MsgFlags = gosnmp.AuthNoPriv
		default:
			c.MsgFlags = gosnmp.NoAuthNoPriv
		}

		c.SecurityModel = gosnmp.UserSecurityModel
		c.ContextName = opts.ContextName

		c.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 opts.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: opts.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        opts.PrivKey,
		}
	}

	return c, nil
}

func classifySNMPError(err error) string {
	switch {
	case errors.Is(err, gosnmp.ErrWrongDigest),
		errors.Is(err, gosnmp.ErrDecryption),
		errors.Is(err, gosnmp.ErrUnknownUsername),
		errors.Is(err, gosnmp.ErrUnknownSecurityLevel),
		errors.Is(err, gosnmp.ErrUnknownEngineID):
		return failureCredential
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(strings.ToLower(err.Error()), "timeout"):
		return failureTimeout
	case errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH):
		return failureUnreachable
	default:
		return failureUnknown
	}
}
