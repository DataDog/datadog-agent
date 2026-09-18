// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// toProbeOptions maps one request to probe options. Its error is answered with
// a 400, so it must say what is wrong with the request.
func toProbeOptions(req connectivity.Request, ping pingprobe.Capability) (probe.Options, error) {
	var opts probe.Options

	for _, check := range req.Checks {
		switch check {
		case connectivity.CheckPing:
			if req.PingOptions == nil {
				return probe.Options{}, fmt.Errorf("%w: options are required for ping", connectivity.ErrInvalidRequest)
			}

			useRawSocket := ping.UseRawSocket
			if req.PingOptions.UseRawSocket != nil {
				useRawSocket = *req.PingOptions.UseRawSocket
			}
			opts.Ping = &pingprobe.Options{
				Count:        req.PingOptions.Count,
				Interval:     time.Duration(req.PingOptions.IntervalMs) * time.Millisecond,
				Timeout:      time.Duration(req.PingOptions.TimeoutMs) * time.Millisecond,
				UseRawSocket: useRawSocket,
			}
		case connectivity.CheckSNMP:
			if req.SNMPOptions == nil {
				return probe.Options{}, fmt.Errorf("%w: options are required for SNMP", connectivity.ErrInvalidRequest)
			}
			if req.SNMPOptions.Port < 1 || req.SNMPOptions.Port > 65535 {
				return probe.Options{}, fmt.Errorf("%w: SNMP port %d out of range (expected 1-65535)", connectivity.ErrInvalidRequest, req.SNMPOptions.Port)
			}
			if len(req.Credentials) == 0 {
				continue
			}

			snmp := &snmpprobe.Options{
				Port:        uint16(req.SNMPOptions.Port),
				Timeout:     time.Duration(req.SNMPOptions.TimeoutMs) * time.Millisecond,
				Retries:     req.SNMPOptions.Retries,
				Credentials: toSNMPCredentials(req.Credentials),
			}
			if err := snmp.Validate(); err != nil {
				return probe.Options{}, fmt.Errorf("%w: %s", connectivity.ErrInvalidRequest, err.Error())
			}
			opts.SNMP = snmp
		default:
			return probe.Options{}, fmt.Errorf("%w: unsupported check: '%s'", connectivity.ErrInvalidRequest, check)
		}
	}

	return opts, nil
}

func toSNMPCredentials(creds []connectivity.SNMPCredential) []snmpprobe.Credential {
	out := make([]snmpprobe.Credential, 0, len(creds))
	for _, c := range creds {
		out = append(out, snmpprobe.Credential{
			ID:              c.ID,
			Version:         c.Version,
			Community:       c.Community,
			User:            c.User,
			AuthProtocol:    c.AuthProtocol,
			AuthKey:         c.AuthKey,
			PrivProtocol:    c.PrivProtocol,
			PrivKey:         c.PrivKey,
			ContextName:     c.ContextName,
			ContextEngineID: c.ContextEngineID,
		})
	}
	return out
}

// toConnectivityResult maps the probe results back to the endpoint's response.
func toConnectivityResult(results []probe.Result) connectivity.Result {
	devices := make([]connectivity.DeviceResult, 0, len(results))
	for _, r := range results {
		d := connectivity.DeviceResult{IPAddress: r.Target}
		if r.Ping != nil {
			d.PingResult = &connectivity.PingResult{
				CheckResult:   toCheckResult(r.Ping.Success, r.Ping.RTT, r.Ping.Error),
				FailureReason: r.Ping.FailureReason,
			}
		}
		if r.SNMP != nil {
			d.SNMPResult = &connectivity.SNMPResult{
				CheckResult:   toCheckResult(r.SNMP.Success, r.SNMP.RTT, r.SNMP.Error),
				FailureReason: r.SNMP.FailureReason,
				CredID:        r.SNMP.CredID,
				SysName:       r.SNMP.SysName,
			}
		}
		devices = append(devices, d)
	}
	return connectivity.Result{Devices: devices}
}

func toCheckResult(success bool, rtt time.Duration, errMsg string) connectivity.CheckResult {
	res := connectivity.CheckResult{Success: success, Error: errMsg}
	if success {
		ms := rtt.Milliseconds()
		res.RttMs = &ms
	}
	return res
}
