// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmpprobe

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

func buildClient(ctx context.Context, target string, opts Options, cred Credential) (*gosnmp.GoSNMP, error) {
	c := &gosnmp.GoSNMP{
		Context:   ctx,
		Target:    target,
		Port:      opts.Port,
		Transport: "udp",
		Timeout:   opts.Timeout,
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
		return nil, fmt.Errorf("unknown SNMP version '%s' (expected 1, 2c, or 3)", cred.Version)
	}

	return c, nil
}

func mapError(err error) string {
	switch {
	case errors.Is(err, gosnmp.ErrWrongDigest):
		return failure.AuthenticationFailed
	case errors.Is(err, gosnmp.ErrDecryption):
		return failure.DecryptionFailed
	case errors.Is(err, gosnmp.ErrUnknownUsername):
		return failure.UnknownUser
	case errors.Is(err, gosnmp.ErrUnknownSecurityLevel):
		return failure.UnsupportedSecurityLevel
	case errors.Is(err, gosnmp.ErrUnknownEngineID):
		return failure.UnknownEngineID
	case errors.Is(err, context.DeadlineExceeded),
		strings.Contains(strings.ToLower(err.Error()), "timeout"):
		return failure.Timeout
	case errors.Is(err, syscall.ECONNREFUSED):
		return failure.ConnectionRefused
	case errors.Is(err, syscall.EHOSTUNREACH):
		return failure.HostUnreachable
	case errors.Is(err, syscall.ENETUNREACH):
		return failure.NetworkUnreachable
	default:
		return failure.Unknown
	}
}
