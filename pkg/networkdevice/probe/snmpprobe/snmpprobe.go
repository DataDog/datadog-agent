// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package snmpprobe probes one address with SNMP.
package snmpprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/failure"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

const oidSysName = "1.3.6.1.2.1.1.5.0"

// ErrInvalidOptions is wrapped by every error Validate returns.
var ErrInvalidOptions = errors.New("invalid snmp options")

// Credential is one set of SNMP authentication material.
type Credential struct {
	ID              string
	Version         string
	Community       string
	User            string
	AuthProtocol    string
	AuthKey         string
	PrivProtocol    string
	PrivKey         string
	ContextName     string
	ContextEngineID string
}

// Options is one SNMP probe's configuration.
type Options struct {
	Port        uint16
	Timeout     time.Duration
	Retries     int
	Credentials []Credential
}

// Reading is what the SNMP probe learned about one address.
type Reading struct {
	Success       bool
	RTT           time.Duration
	FailureReason string
	Error         string
	CredID        string
	SysName       string
}

// Validate reports what makes these options unusable. It names a credential id,
// never the material behind it.
func (o Options) Validate() error {
	if o.Port == 0 {
		return fmt.Errorf("%w: port 0 is out of range (expected 1-65535)", ErrInvalidOptions)
	}
	if o.Retries < 0 {
		return fmt.Errorf("%w: retries %d is negative", ErrInvalidOptions, o.Retries)
	}
	if len(o.Credentials) == 0 {
		return fmt.Errorf("%w: at least one credential is required", ErrInvalidOptions)
	}
	for _, c := range o.Credentials {
		if err := c.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (c Credential) validate() error {
	switch c.Version {
	case "1", "2c":
		return nil
	case "3":
		if _, err := gosnmplib.GetAuthProtocol(c.AuthProtocol); err != nil {
			return fmt.Errorf("%w: credential %q: %s", ErrInvalidOptions, c.ID, err.Error())
		}
		if _, err := gosnmplib.GetPrivProtocol(c.PrivProtocol); err != nil {
			return fmt.Errorf("%w: credential %q: %s", ErrInvalidOptions, c.ID, err.Error())
		}
		return nil
	default:
		return fmt.Errorf("%w: credential %q has unknown SNMP version '%s' (expected 1, 2c, or 3)", ErrInvalidOptions, c.ID, c.Version)
	}
}

// Fingerprint is a stable digest of these options. It hashes the credential
// material, so no secret leaves this method.
func (o Options) Fingerprint() string {
	h := sha256.New()
	fmt.Fprintf(h, "port=%d timeout=%s retries=%d\n", o.Port, o.Timeout, o.Retries)

	creds := make([]string, 0, len(o.Credentials))
	for _, c := range o.Credentials {
		creds = append(creds, fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			c.ID, c.Version, c.Community, c.User, c.AuthProtocol, c.AuthKey,
			c.PrivProtocol, c.PrivKey, c.ContextName, c.ContextEngineID))
	}
	sort.Strings(creds)
	for _, c := range creds {
		fmt.Fprintf(h, "cred=%s\n", c)
	}

	return "snmp:" + hex.EncodeToString(h.Sum(nil))
}

// Run probes one address, trying each credential in order until one answers.
// A failure is a Reading, not an error.
func Run(ctx context.Context, target string, opts Options) *Reading {
	var last *Reading
	for _, cred := range opts.Credentials {
		if err := ctx.Err(); err != nil {
			break
		}

		r := try(ctx, target, opts, cred)
		if r.Success {
			return r
		}
		last = r
	}

	if last == nil {
		return &Reading{
			FailureReason: failure.Unknown,
			Error:         fmt.Sprintf("No SNMP credential could be tried for host '%s'", target),
		}
	}
	return last
}

func try(ctx context.Context, target string, opts Options, cred Credential) *Reading {
	c, err := buildClient(ctx, target, opts, cred)
	if err != nil {
		return &Reading{
			FailureReason: failure.Unknown,
			Error:         fmt.Sprintf("Failed to create SNMP client for host '%s': %s", target, err.Error()),
		}
	}

	if err := c.Connect(); err != nil {
		return &Reading{
			FailureReason: mapError(err),
			Error:         fmt.Sprintf("Failed to connect to SNMP host '%s': %s", target, err.Error()),
		}
	}
	defer func() { _ = c.Conn.Close() }()

	start := time.Now()
	packet, err := c.Get([]string{oidSysName})
	if err != nil {
		return &Reading{
			FailureReason: mapError(err),
			Error:         fmt.Sprintf("Failed to fetch device name for host '%s': %s", target, err.Error()),
		}
	}

	r := &Reading{Success: true, RTT: time.Since(start), FailureReason: failure.None, CredID: cred.ID}
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
			r.SysName = strValue
		}
	}
	return r
}
