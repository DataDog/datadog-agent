// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// toProbeOptions maps one request to probe options. Its error is answered with a 400.
func toProbeOptions(req connectivity.Request, ping pingprobe.Capability) (probe.Options, error) {
	var opts probe.Options

	for _, check := range req.Checks {
		switch check {
		case connectivity.CheckPing:
			if req.PingOptions == nil {
				return probe.Options{}, fmt.Errorf("%w: options are required for ping", connectivity.ErrInvalidRequest)
			}

			opts.Ping = &pingprobe.Options{
				Count:        req.PingOptions.Count,
				Interval:     time.Duration(req.PingOptions.IntervalMs) * time.Millisecond,
				Timeout:      time.Duration(req.PingOptions.TimeoutMs) * time.Millisecond,
				UseRawSocket: resolveRawSocket(runtime.GOOS, ping.UseRawSocket, req.PingOptions.UseRawSocket),
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

// toTargetPlans maps a grouped request to its targets, in group order then in
// each group's order, and to each target's probes: a target is tried only with
// the credentials its group names, in the group's order. Every group needs at
// least one target and one credential, a target is in one group only, and every
// credential id must name one of the request's credentials. Its error is
// answered with a 400.
func toTargetPlans(req connectivity.Request, ping pingprobe.Capability) ([]string, map[string]probe.Options, error) {
	if len(req.TargetGroups) == 0 {
		return nil, nil, fmt.Errorf("%w: target groups are empty", connectivity.ErrInvalidRequest)
	}

	creds := make(map[string]connectivity.SNMPCredential, len(req.Credentials))
	for _, c := range req.Credentials {
		if _, dup := creds[c.ID]; dup {
			return nil, nil, fmt.Errorf("%w: credential id '%s' is not unique", connectivity.ErrInvalidRequest, c.ID)
		}
		creds[c.ID] = c
	}

	var targets []string
	plans := make(map[string]probe.Options)
	for i, g := range req.TargetGroups {
		if len(g.TargetIPs) == 0 {
			return nil, nil, fmt.Errorf("%w: target group %d has no target", connectivity.ErrInvalidRequest, i)
		}
		if len(g.CredentialIDs) == 0 {
			return nil, nil, fmt.Errorf("%w: target group %d has no credential", connectivity.ErrInvalidRequest, i)
		}

		groupReq := req
		groupReq.Credentials = make([]connectivity.SNMPCredential, 0, len(g.CredentialIDs))
		named := make(map[string]struct{}, len(g.CredentialIDs))
		for _, id := range g.CredentialIDs {
			c, ok := creds[id]
			if !ok {
				return nil, nil, fmt.Errorf("%w: target group %d names unknown credential '%s'", connectivity.ErrInvalidRequest, i, id)
			}
			if _, dup := named[id]; dup {
				return nil, nil, fmt.Errorf("%w: target group %d names credential '%s' more than once", connectivity.ErrInvalidRequest, i, id)
			}
			named[id] = struct{}{}
			groupReq.Credentials = append(groupReq.Credentials, c)
		}

		opts, err := toProbeOptions(groupReq, ping)
		if err != nil {
			return nil, nil, fmt.Errorf("target group %d: %w", i, err)
		}

		for _, ip := range g.TargetIPs {
			if _, dup := plans[ip]; dup {
				return nil, nil, fmt.Errorf("%w: target '%s' is listed more than once in the target groups", connectivity.ErrInvalidRequest, ip)
			}
			plans[ip] = opts
			targets = append(targets, ip)
		}
	}

	return targets, plans, nil
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
// resolveRawSocket applies the request's socket-type override only on the
// platform that offers a choice. Windows has no UDP pinger and macOS no raw
// one, so there the detected value is the only one that works.
func resolveRawSocket(goos string, detected bool, override *bool) bool {
	if goos == "linux" && override != nil {
		return *override
	}
	return detected
}

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
		ms := int64(math.Round(float64(rtt) / float64(time.Millisecond)))
		res.RttMs = &ms
	}
	return res
}
