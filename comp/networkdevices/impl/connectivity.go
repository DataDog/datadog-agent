// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gosnmp/gosnmp"
	"golang.org/x/sync/errgroup"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/pinger"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const (
	oidSysName = "1.3.6.1.2.1.1.5.0"
)

func (c *networkDevicesImpl) CheckConnectivity(ctx context.Context, req connectivity.Request) (connectivity.Result, error) {
	workers := req.Workers
	if workers < 1 {
		workers = 1
	}

	devices := make([]connectivity.DeviceResult, len(req.Targets))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	for i, ip := range req.Targets {
		g.Go(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}

			dr := connectivity.DeviceResult{IPAddress: ip}
			for _, check := range req.Checks {
				switch check {
				case connectivity.CheckPing:
					res, err := runPing(ip, req.PingOptions)
					if err != nil {
						return fmt.Errorf("failed to run ping check for host '%s': %w", ip, err)
					}

					dr.PingResult = res
				case connectivity.CheckSNMP:
					res, err := runSNMP(ctx, ip, req.SNMPOptions, req.Credentials)
					if err != nil {
						return fmt.Errorf("failed to run SNMP check for host '%s': %w", ip, err)
					}

					dr.SNMPResult = res
				default:
					return fmt.Errorf("%w: unsupported check: '%s'", connectivity.ErrInvalidRequest, check)
				}
			}
			devices[i] = dr
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return connectivity.Result{}, err
	}
	return connectivity.Result{Devices: devices}, nil
}

func runPing(host string, opts *connectivity.PingOptions) (*connectivity.PingResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("%w: options are required for ping", connectivity.ErrInvalidRequest)
	}

	p, err := buildPinger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create pinger: %w", err)
	}

	res, err := p.Ping(host)
	if err != nil {
		return &connectivity.PingResult{
			CheckResult:   connectivity.CheckResult{Error: fmt.Sprintf("Failed to reach host '%s': %s", host, err.Error())},
			FailureReason: connectivity.FailureUnreachable,
		}, nil
	}
	if res == nil || !res.CanConnect {
		return &connectivity.PingResult{
			CheckResult:   connectivity.CheckResult{Error: fmt.Sprintf("Failed to connect to host '%s'", host)},
			FailureReason: connectivity.FailureUnreachable,
		}, nil
	}

	rtt := res.AvgRtt.Milliseconds()
	return &connectivity.PingResult{
		CheckResult:   connectivity.CheckResult{Success: true, RttMs: &rtt},
		FailureReason: connectivity.FailureNone,
	}, nil
}

func buildPinger(opts *connectivity.PingOptions) (pinger.Pinger, error) {
	var useRawSocket bool
	switch runtime.GOOS {
	case "windows":
		useRawSocket = true
	case "darwin":
		useRawSocket = false
	case "linux":
		if opts.UseRawSocket != nil {
			useRawSocket = *opts.UseRawSocket
		}
	default:
		return nil, fmt.Errorf("ping is not supported on %s", runtime.GOOS)
	}

	return pinger.New(pinger.Config{
		UseRawSocket: useRawSocket,
		Count:        opts.Count,
		Interval:     time.Duration(opts.IntervalMs) * time.Millisecond,
		Timeout:      time.Duration(opts.TimeoutMs) * time.Millisecond,
	})
}

func runSNMP(ctx context.Context, host string, opts *connectivity.SNMPOptions, creds []connectivity.SNMPCredential) (*connectivity.SNMPResult, error) {
	if opts == nil {
		return nil, fmt.Errorf("%w: options are required for SNMP", connectivity.ErrInvalidRequest)
	}
	if opts.Port < 1 || opts.Port > 65535 {
		return nil, fmt.Errorf("%w: SNMP port %d out of range (expected 1-65535)", connectivity.ErrInvalidRequest, opts.Port)
	}

	var lastResult *connectivity.SNMPResult
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

func trySNMPCredential(ctx context.Context, host string, opts *connectivity.SNMPOptions, cred connectivity.SNMPCredential) (*connectivity.SNMPResult, error) {
	c, err := buildSNMPClient(ctx, host, opts, cred)
	if err != nil {
		return nil, fmt.Errorf("failed to create SNMP client: %w", err)
	}

	err = c.Connect()
	if err != nil {
		return &connectivity.SNMPResult{
			CheckResult:   connectivity.CheckResult{Error: fmt.Sprintf("Failed to connect to SNMP host '%s': %s", host, err.Error())},
			FailureReason: mapSNMPError(err),
		}, nil
	}
	defer func() { _ = c.Conn.Close() }()

	startTime := time.Now()
	packet, err := c.Get([]string{oidSysName})
	if err != nil {
		return &connectivity.SNMPResult{
			CheckResult:   connectivity.CheckResult{Error: fmt.Sprintf("Failed to fetch device name for host '%s': %s", host, err.Error())},
			FailureReason: mapSNMPError(err),
		}, nil
	}
	rtt := time.Since(startTime).Milliseconds()

	res := &connectivity.SNMPResult{
		CheckResult:   connectivity.CheckResult{Success: true, RttMs: &rtt},
		FailureReason: connectivity.FailureNone,
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

func buildSNMPClient(ctx context.Context, host string, opts *connectivity.SNMPOptions, cred connectivity.SNMPCredential) (*gosnmp.GoSNMP, error) {
	c := &gosnmp.GoSNMP{
		Context:   ctx,
		Target:    host,
		Port:      uint16(opts.Port),
		Transport: "udp",
		Timeout:   time.Duration(opts.TimeoutMs) * time.Millisecond,
		Retries:   opts.Retries,
	}

	switch cred.Version {
	case "1":
		c.Version = gosnmp.Version1
		c.Community = cred.Community
	case "2c":
		c.Version = gosnmp.Version2c
		c.Community = cred.Community
	case "3":
		c.Version = gosnmp.Version3

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
	default:
		return nil, fmt.Errorf("%w: unknown SNMP version '%s' (expected 1, 2c, or 3)", connectivity.ErrInvalidRequest, cred.Version)
	}

	return c, nil
}

func mapSNMPError(err error) string {
	switch {
	case errors.Is(err, gosnmp.ErrWrongDigest):
		return connectivity.FailureAuthenticationFailed
	case errors.Is(err, gosnmp.ErrDecryption):
		return connectivity.FailureDecryptionFailed
	case errors.Is(err, gosnmp.ErrUnknownUsername):
		return connectivity.FailureUnknownUser
	case errors.Is(err, gosnmp.ErrUnknownSecurityLevel):
		return connectivity.FailureUnsupportedSecurityLevel
	case errors.Is(err, gosnmp.ErrUnknownEngineID):
		return connectivity.FailureUnknownEngineID
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(strings.ToLower(err.Error()), "timeout"):
		return connectivity.FailureTimeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return connectivity.FailureConnectionRefused
	case errors.Is(err, syscall.EHOSTUNREACH):
		return connectivity.FailureHostUnreachable
	case errors.Is(err, syscall.ENETUNREACH):
		return connectivity.FailureNetworkUnreachable
	default:
		return connectivity.FailureUnknown
	}
}
