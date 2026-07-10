// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	pingInterval = 100 * time.Millisecond

	oidSysName = "1.3.6.1.2.1.1.5.0"

	checkPing = "ping"
	checkSNMP = "snmp"

	failureNone                     = "none"
	failureUnknown                  = "unknown"
	failureUnreachable              = "unreachable"
	failureTimeout                  = "timeout"
	failureConnectionRefused        = "connection_refused"
	failureHostUnreachable          = "host_unreachable"
	failureNetworkUnreachable       = "network_unreachable"
	failureAuthenticationFailed     = "authentication_failed"
	failureDecryptionFailed         = "decryption_failed"
	failureUnknownUser              = "unknown_user"
	failureUnsupportedSecurityLevel = "unsupported_security_level"
	failureUnknownEngineID          = "unknown_engine_id"
)

type PingOptions struct {
	Count     int `json:"count"`
	TimeoutMs int `json:"timeoutMs"`
}

type SNMPOptions struct {
	Port      int `json:"port"`
	TimeoutMs int `json:"timeoutMs"`
	Retries   int `json:"retries"`
}

type ConnectivityCheckRequest struct {
	TargetIPs            []string                            `json:"targetIPs"`
	Checks               []string                            `json:"checks"`
	PingOptions          *PingOptions                        `json:"pingOptions,omitempty"`
	SNMPOptions          *SNMPOptions                        `json:"snmpOptions,omitempty"`
	EncryptedCredentials string                              `json:"encryptedCredentials"`
	EncryptionContext    encryptioncontext.EncryptionContext `json:"encryptionContext"`
}

type SNMPCredential struct {
	ID              string `json:"id"`
	Version         string `json:"version"`
	Community       string `json:"community,omitempty"`
	User            string `json:"user,omitempty"`
	AuthProtocol    string `json:"authProtocol,omitempty"`
	AuthKey         string `json:"authKey,omitempty"`
	PrivProtocol    string `json:"privProtocol,omitempty"`
	PrivKey         string `json:"privKey,omitempty"`
	ContextName     string `json:"contextName,omitempty"`
	ContextEngineID string `json:"contextEngineId,omitempty"`
}

type secretInputs struct {
	SNMP []SNMPCredential `json:"snmp"`
}

type CheckResult struct {
	Success bool   `json:"success"`
	RttMs   *int64 `json:"rttMs,omitempty"`
	Error   string `json:"error,omitempty"`
}

type PingResult struct {
	CheckResult
	FailureReason string `json:"failureReason"`
}

type SNMPResult struct {
	CheckResult
	FailureReason string `json:"failureReason"`
	CredID        string `json:"credID,omitempty"`
	SysName       string `json:"sysName,omitempty"`
}

type DeviceResult struct {
	IPAddress  string      `json:"ipAddress"`
	PingResult *PingResult `json:"pingResult,omitempty"`
	SNMPResult *SNMPResult `json:"snmpResult,omitempty"`
}

type ConnectivityCheckResult struct {
	Devices []DeviceResult `json:"devices"`
}

type ConnectivityCheckHandler struct {
	encryptionStore *encryptioncontext.Store
}

func NewConnectivityCheckHandler(encryptionStore *encryptioncontext.Store) *ConnectivityCheckHandler {
	return &ConnectivityCheckHandler{encryptionStore: encryptionStore}
}

func (h *ConnectivityCheckHandler) Run(ctx context.Context, task *types.Task, _ *privateconnection.PrivateCredentials) (interface{}, error) {
	req, err := types.ExtractInputs[ConnectivityCheckRequest](task)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connectivityCheck inputs: %w", err)
	}

	var secrets secretInputs
	if req.EncryptedCredentials != "" {
		secrets, err = encryptioncontext.DecryptInto[secretInputs](h.encryptionStore, req.EncryptionContext, req.EncryptedCredentials)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt secret inputs: %w", err)
		}
	}

	res, err := runChecks(ctx, req, secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to run connectivity checks: %w", err)
	}

	return res, nil
}

func runChecks(ctx context.Context, req ConnectivityCheckRequest, secrets secretInputs) (ConnectivityCheckResult, error) {
	devices := make([]DeviceResult, 0, len(req.TargetIPs))
	for _, ip := range req.TargetIPs {
		if err := ctx.Err(); err != nil {
			return ConnectivityCheckResult{}, err
		}

		dr := DeviceResult{IPAddress: ip}
		for _, c := range req.Checks {
			switch c {
			case checkPing:
				res, err := runPing(ip, req.PingOptions)
				if err != nil {
					return ConnectivityCheckResult{}, fmt.Errorf("failed to run ping check for host '%s': %w", ip, err)
				}

				dr.PingResult = res
			case checkSNMP:
				res, err := runSNMP(ctx, ip, req.SNMPOptions, secrets.SNMP)
				if err != nil {
					return ConnectivityCheckResult{}, fmt.Errorf("failed to run SNMP check for host '%s': %w", ip, err)
				}

				dr.SNMPResult = res
			}
		}
		devices = append(devices, dr)
	}

	return ConnectivityCheckResult{Devices: devices}, nil
}

func runPing(host string, opts *PingOptions) (*PingResult, error) {
	if opts == nil {
		return nil, errors.New("options are required for ping")
	}

	p, err := buildPinger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create pinger: %w", err)
	}

	res, err := p.Ping(host)
	if err != nil {
		return &PingResult{
			CheckResult:   CheckResult{Error: fmt.Sprintf("Failed to reach host '%s': %s", host, err.Error())},
			FailureReason: failureUnreachable,
		}, nil
	}
	if res == nil || !res.CanConnect {
		return &PingResult{
			CheckResult:   CheckResult{Error: fmt.Sprintf("Failed to connect to host '%s'", host)},
			FailureReason: failureUnreachable,
		}, nil
	}

	rtt := res.AvgRtt.Milliseconds()
	return &PingResult{
		CheckResult:   CheckResult{Success: true, RttMs: &rtt},
		FailureReason: failureNone,
	}, nil
}

func buildPinger(opts *PingOptions) (pinger.Pinger, error) {
	var useRawSocket bool
	switch runtime.GOOS {
	case "windows":
		useRawSocket = true
	case "darwin":
		useRawSocket = false
	default:
		useRawSocket = true
	}

	return pinger.New(pinger.Config{
		UseRawSocket: useRawSocket,
		Count:        opts.Count,
		Timeout:      time.Duration(opts.TimeoutMs) * time.Millisecond,
		Interval:     pingInterval,
	})
}

func runSNMP(ctx context.Context, host string, opts *SNMPOptions, creds []SNMPCredential) (*SNMPResult, error) {
	if opts == nil {
		return nil, errors.New("options are required for SNMP")
	}

	var lastResult *SNMPResult
	for _, cred := range creds {
		res, err := trySNMPCredential(ctx, host, opts, cred)
		if err != nil {
			return nil, err
		}

		if res.Success {
			return res, nil
		}

		lastResult = res
	}

	return lastResult, nil
}

func trySNMPCredential(ctx context.Context, host string, opts *SNMPOptions, cred SNMPCredential) (*SNMPResult, error) {
	c, err := buildSNMPClient(ctx, host, opts, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create SNMP client: %w", err)
	}

	err = c.Connect()
	if err != nil {
		return &SNMPResult{
			CheckResult:   CheckResult{Error: fmt.Sprintf("Failed to connect to SNMP host '%s': %s", host, err.Error())},
			FailureReason: mapSNMPError(err),
		}, nil
	}
	defer func() { _ = c.Conn.Close() }()

	startTime := time.Now()
	packet, err := c.Get([]string{oidSysName})
	if err != nil {
		return &SNMPResult{
			CheckResult:   CheckResult{Error: fmt.Sprintf("Failed to fetch device name for host '%s': %s", host, err.Error())},
			FailureReason: mapSNMPError(err),
		}, nil
	}
	rtt := time.Since(startTime).Milliseconds()

	res := &SNMPResult{
		CheckResult:   CheckResult{Success: true, RttMs: &rtt},
		FailureReason: failureNone,
		CredID:        cred.ID,
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

	return res, nil
}

func buildSNMPClient(ctx context.Context, host string, opts *SNMPOptions, cred SNMPCredential) (*gosnmp.GoSNMP, error) {
	var ver gosnmp.SnmpVersion
	switch cred.Version {
	case "1":
		ver = gosnmp.Version1
	case "2c":
		ver = gosnmp.Version2c
	case "3":
		ver = gosnmp.Version3
	default:
		return nil, fmt.Errorf("unknown SNMP version '%s' (expected 1, 2c, or 3)", cred.Version)
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
		c.Community = cred.Community
	}
	if ver == gosnmp.Version3 {
		authProtocol, err := gosnmplib.GetAuthProtocol(cred.AuthProtocol)
		if err != nil {
			return nil, err
		}

		privProtocol, err := gosnmplib.GetPrivProtocol(cred.PrivProtocol)
		if err != nil {
			return nil, err
		}

		switch {
		case cred.PrivKey != "":
			c.MsgFlags = gosnmp.AuthPriv
		case cred.AuthKey != "":
			c.MsgFlags = gosnmp.AuthNoPriv
		default:
			c.MsgFlags = gosnmp.NoAuthNoPriv
		}

		c.SecurityModel = gosnmp.UserSecurityModel
		c.ContextName = cred.ContextName
		c.ContextEngineID = cred.ContextEngineID

		c.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 cred.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: cred.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        cred.PrivKey,
		}
	}

	return c, nil
}

func mapSNMPError(err error) string {
	switch {
	case errors.Is(err, gosnmp.ErrWrongDigest):
		return failureAuthenticationFailed
	case errors.Is(err, gosnmp.ErrDecryption):
		return failureDecryptionFailed
	case errors.Is(err, gosnmp.ErrUnknownUsername):
		return failureUnknownUser
	case errors.Is(err, gosnmp.ErrUnknownSecurityLevel):
		return failureUnsupportedSecurityLevel
	case errors.Is(err, gosnmp.ErrUnknownEngineID):
		return failureUnknownEngineID
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(strings.ToLower(err.Error()), "timeout"):
		return failureTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return failureConnectionRefused
	case errors.Is(err, syscall.EHOSTUNREACH):
		return failureHostUnreachable
	case errors.Is(err, syscall.ENETUNREACH):
		return failureNetworkUnreachable
	default:
		return failureUnknown
	}
}
