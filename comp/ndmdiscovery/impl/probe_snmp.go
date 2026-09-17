// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

const kindSNMP = "snmp"

// Defaults and bounds of the snmp probe options. An out-of-bounds value falls
// back to its default.
const (
	defaultSNMPPort      = 161
	defaultSNMPTimeoutMs = 2000
	defaultSNMPRetries   = 1
	maxSNMPTimeoutMs     = 60_000
	maxSNMPRetries       = 10
)

// snmpOptions is the snmp block of a range's probes object.
type snmpOptions struct {
	CredentialIDs []string `json:"credential_ids"`
	Port          int      `json:"port"`
	TimeoutMs     int      `json:"timeout_ms"`
	Retries       *int     `json:"retries"`
}

var _ probe = (*snmpProbe)(nil)

// snmpProbe scans a range with SNMP.
type snmpProbe struct {
	store credentialStore
	log   log.Component
}

// newSNMPProbe builds the snmp probe over the store holding the SNMP credentials.
func newSNMPProbe(store credentialStore, logger log.Component) *snmpProbe {
	return &snmpProbe{store: store, log: logger}
}

func (p *snmpProbe) kind() string { return kindSNMP }

// detect is a no-op: SNMP needs no privilege this agent might lack.
func (p *snmpProbe) detect(context.Context) {}

func (p *snmpProbe) available() bool { return true }

func (p *snmpProbe) parse(raw json.RawMessage) (probeConfig, error) {
	var opts snmpOptions
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, fmt.Errorf("the snmp options are not an object: %w", err)
	}
	if len(opts.CredentialIDs) == 0 {
		return nil, errors.New("credential_ids must hold at least one credential")
	}

	resolved := connectivity.SNMPOptions{
		Port:      defaultSNMPPort,
		TimeoutMs: defaultSNMPTimeoutMs,
		Retries:   defaultSNMPRetries,
	}
	if opts.Port != 0 {
		if opts.Port >= 1 && opts.Port <= 65535 {
			resolved.Port = opts.Port
		} else {
			p.log.Warnf("ndmdiscovery: snmp port %d is out of range (expected 1-65535), using %d", opts.Port, defaultSNMPPort)
		}
	}
	if opts.TimeoutMs != 0 {
		if opts.TimeoutMs >= 1 && opts.TimeoutMs <= maxSNMPTimeoutMs {
			resolved.TimeoutMs = opts.TimeoutMs
		} else {
			p.log.Warnf("ndmdiscovery: snmp timeout_ms %d is out of range (expected 1-%d), using %d", opts.TimeoutMs, maxSNMPTimeoutMs, defaultSNMPTimeoutMs)
		}
	}
	if opts.Retries != nil {
		// An explicit zero is a legitimate do-not-retry setting.
		if *opts.Retries >= 0 && *opts.Retries <= maxSNMPRetries {
			resolved.Retries = *opts.Retries
		} else {
			p.log.Warnf("ndmdiscovery: snmp retries %d is out of range (expected 0-%d), using %d", *opts.Retries, maxSNMPRetries, defaultSNMPRetries)
		}
	}

	return &snmpConfig{store: p.store, credentialIDs: opts.CredentialIDs, options: resolved}, nil
}

var _ probeConfig = (*snmpConfig)(nil)

// snmpConfig is the snmp probe's configuration for one range.
type snmpConfig struct {
	store         credentialStore
	credentialIDs []string
	options       connectivity.SNMPOptions
}

func (c *snmpConfig) kind() string { return kindSNMP }

func (c *snmpConfig) prepare() (probeRun, error) {
	creds, err := resolveCredentials(c.store, c.credentialIDs)
	if err != nil {
		return nil, err
	}
	return &snmpRun{options: c.options, credentials: creds}, nil
}

var _ probeRun = (*snmpRun)(nil)

// snmpRun is the snmp probe's contribution to one cycle.
type snmpRun struct {
	options     connectivity.SNMPOptions
	credentials []connectivity.SNMPCredential
}

// fingerprint hashes the options and the resolved credentials, so no secret
// leaves this method.
func (r *snmpRun) fingerprint() string {
	h := sha256.New()
	fmt.Fprintf(h, "port=%d timeout=%d retries=%d\n", r.options.Port, r.options.TimeoutMs, r.options.Retries)

	creds := make([]string, 0, len(r.credentials))
	for _, c := range r.credentials {
		creds = append(creds, fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%s",
			c.ID, c.Version, c.Community, c.User, c.AuthProtocol, c.AuthKey,
			c.PrivProtocol, c.PrivKey, c.ContextName, c.ContextEngineID))
	}
	sort.Strings(creds)
	for _, c := range creds {
		fmt.Fprintf(h, "cred=%s\n", c)
	}

	return kindSNMP + ":" + hex.EncodeToString(h.Sum(nil))
}

func (r *snmpRun) apply(req *connectivity.Request) {
	req.Checks = append(req.Checks, connectivity.CheckSNMP)
	options := r.options
	req.SNMPOptions = &options
	req.Credentials = r.credentials
}

func (r *snmpRun) read(d connectivity.DeviceResult) *probeReading {
	if d.SNMPResult == nil {
		return nil
	}

	reading := &probeReading{Result: metadata.ProbeResult{
		Kind:   kindSNMP,
		Status: statusString(d.SNMPResult.Success),
		RttMs:  d.SNMPResult.RttMs,
	}}
	if d.SNMPResult.Success {
		reading.Result.CredID = d.SNMPResult.CredID
		reading.Name = d.SNMPResult.SysName
	} else {
		reading.Result.FailureReason = d.SNMPResult.FailureReason
	}
	return reading
}

// resolveCredentials maps a range's credential ids to credentials, keeping the
// configured order so the most likely credential is tried first.
func resolveCredentials(store credentialStore, ids []string) ([]connectivity.SNMPCredential, error) {
	if len(ids) == 0 {
		return nil, errors.New("the range references no credentials")
	}

	available, err := store.Load()
	if err != nil {
		return nil, err
	}

	creds := make([]connectivity.SNMPCredential, 0, len(ids))
	for _, id := range ids {
		c, ok := available[id]
		if !ok {
			return nil, fmt.Errorf("credential %q is not available on this agent", id)
		}
		if err := credentials.Validate(c); err != nil {
			return nil, err
		}
		creds = append(creds, connectivity.SNMPCredential{
			ID:              c.ID,
			Version:         c.SNMPVersion,
			Community:       c.CommunityString,
			User:            c.User,
			AuthProtocol:    c.AuthProtocol,
			AuthKey:         c.AuthKey,
			PrivProtocol:    c.PrivProtocol,
			PrivKey:         c.PrivKey,
			ContextName:     c.ContextName,
			ContextEngineID: c.ContextEngineID,
		})
	}
	return creds, nil
}
