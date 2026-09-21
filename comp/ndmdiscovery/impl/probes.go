// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/ndm/credentials"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/snmpprobe"
)

// The probe kinds a range can ask for, as they appear in the RC document.
const (
	kindPing = "ping"
	kindSNMP = "snmp"
)

// Defaults and bounds of the ping probe options. An out-of-bounds value falls
// back to its default.
const (
	defaultPingCount      = 1
	defaultPingIntervalMs = 1000
	defaultPingTimeoutMs  = 1000
	maxPingCount          = 10
	maxPingIntervalMs     = 60_000
	maxPingTimeoutMs      = 60_000
)

// Defaults and bounds of the snmp probe options. An out-of-bounds value falls
// back to its default.
const (
	defaultSNMPPort      = 161
	defaultSNMPTimeoutMs = 2000
	defaultSNMPRetries   = 1
	maxSNMPTimeoutMs     = 60_000
	maxSNMPRetries       = 10
)

// pingJSON is the ping block of a range's probes object.
type pingJSON struct {
	Count      int `json:"count"`
	IntervalMs int `json:"interval_ms"`
	TimeoutMs  int `json:"timeout_ms"`
}

// snmpJSON is the snmp block of a range's probes object.
type snmpJSON struct {
	CredentialIDs []string `json:"credential_ids"`
	Port          int      `json:"port"`
	TimeoutMs     int      `json:"timeout_ms"`
	Retries       *int     `json:"retries"`
}

// snmpParams is the snmp probe's per-range configuration. The credential ids
// are resolved once per cycle, so a rotation lands without an RC redelivery.
type snmpParams struct {
	Port          uint16
	Timeout       time.Duration
	Retries       int
	CredentialIDs []string
}

// probeParams is one range's usable probes. A nil field is a probe that does
// not run.
type probeParams struct {
	Ping *pingprobe.Options
	SNMP *snmpParams
}

// credentialStore reads the credentials of one kind from the Agent configuration.
type credentialStore interface {
	Load() (map[string]credentials.Credential, error)
}

// parseProbes validates one range's probes object. An unknown, unavailable or
// rejecting probe is dropped and logged, so the rest of the range still runs.
func parseProbes(rangeID string, raw map[string]json.RawMessage, ping pingprobe.Capability, logger log.Component) probeParams {
	var params probeParams

	for kind, body := range raw {
		switch kind {
		case kindPing:
			if !ping.Available {
				logger.Warnf("ndmdiscovery: range %s: dropping the ping probe, it is not available on this agent: %s", rangeID, ping.Reason)
				continue
			}
			opts, err := parsePing(body, ping, logger)
			if err != nil {
				logger.Warnf("ndmdiscovery: range %s: dropping the ping probe: %v", rangeID, err)
				continue
			}
			params.Ping = opts
		case kindSNMP:
			opts, err := parseSNMP(body, logger)
			if err != nil {
				logger.Warnf("ndmdiscovery: range %s: dropping the snmp probe: %v", rangeID, err)
				continue
			}
			params.SNMP = opts
		default:
			logger.Warnf("ndmdiscovery: range %s: dropping the unknown probe %q", rangeID, kind)
		}
	}

	return params
}

func parsePing(body json.RawMessage, ping pingprobe.Capability, logger log.Component) (*pingprobe.Options, error) {
	var opts pingJSON
	if err := json.Unmarshal(body, &opts); err != nil {
		return nil, fmt.Errorf("the ping options are not an object: %w", err)
	}

	count := defaultPingCount
	if opts.Count != 0 {
		if opts.Count >= 1 && opts.Count <= maxPingCount {
			count = opts.Count
		} else {
			logger.Warnf("ndmdiscovery: ping count %d is out of range (expected 1-%d), using %d", opts.Count, maxPingCount, defaultPingCount)
		}
	}

	intervalMs := defaultPingIntervalMs
	if opts.IntervalMs != 0 {
		if opts.IntervalMs >= 1 && opts.IntervalMs <= maxPingIntervalMs {
			intervalMs = opts.IntervalMs
		} else {
			logger.Warnf("ndmdiscovery: ping interval_ms %d is out of range (expected 1-%d), using %d", opts.IntervalMs, maxPingIntervalMs, defaultPingIntervalMs)
		}
	}

	timeoutMs := defaultPingTimeoutMs
	if opts.TimeoutMs != 0 {
		if opts.TimeoutMs >= 1 && opts.TimeoutMs <= maxPingTimeoutMs {
			timeoutMs = opts.TimeoutMs
		} else {
			logger.Warnf("ndmdiscovery: ping timeout_ms %d is out of range (expected 1-%d), using %d", opts.TimeoutMs, maxPingTimeoutMs, defaultPingTimeoutMs)
		}
	}

	return &pingprobe.Options{
		Count:        count,
		Interval:     time.Duration(intervalMs) * time.Millisecond,
		Timeout:      time.Duration(timeoutMs) * time.Millisecond,
		UseRawSocket: ping.UseRawSocket,
	}, nil
}

func parseSNMP(body json.RawMessage, logger log.Component) (*snmpParams, error) {
	var opts snmpJSON
	if err := json.Unmarshal(body, &opts); err != nil {
		return nil, fmt.Errorf("the snmp options are not an object: %w", err)
	}
	if len(opts.CredentialIDs) == 0 {
		return nil, errors.New("credential_ids must hold at least one credential")
	}

	port := defaultSNMPPort
	if opts.Port != 0 {
		if opts.Port >= 1 && opts.Port <= 65535 {
			port = opts.Port
		} else {
			logger.Warnf("ndmdiscovery: snmp port %d is out of range (expected 1-65535), using %d", opts.Port, defaultSNMPPort)
		}
	}

	timeoutMs := defaultSNMPTimeoutMs
	if opts.TimeoutMs != 0 {
		if opts.TimeoutMs >= 1 && opts.TimeoutMs <= maxSNMPTimeoutMs {
			timeoutMs = opts.TimeoutMs
		} else {
			logger.Warnf("ndmdiscovery: snmp timeout_ms %d is out of range (expected 1-%d), using %d", opts.TimeoutMs, maxSNMPTimeoutMs, defaultSNMPTimeoutMs)
		}
	}

	retries := defaultSNMPRetries
	if opts.Retries != nil {
		// An explicit zero is a legitimate do-not-retry setting.
		if *opts.Retries >= 0 && *opts.Retries <= maxSNMPRetries {
			retries = *opts.Retries
		} else {
			logger.Warnf("ndmdiscovery: snmp retries %d is out of range (expected 0-%d), using %d", *opts.Retries, maxSNMPRetries, defaultSNMPRetries)
		}
	}

	return &snmpParams{
		Port:          uint16(port),
		Timeout:       time.Duration(timeoutMs) * time.Millisecond,
		Retries:       retries,
		CredentialIDs: opts.CredentialIDs,
	}, nil
}

// resolve builds this cycle's probe options. A probe whose credentials cannot
// be resolved is dropped, and its reason is returned for the caller to log.
func (p probeParams) resolve(store credentialStore) (probe.Options, []string) {
	var (
		opts    probe.Options
		dropped []string
	)

	if p.Ping != nil {
		ping := *p.Ping
		opts.Ping = &ping
	}

	if p.SNMP != nil {
		creds, err := resolveSNMPCredentials(store, p.SNMP.CredentialIDs)
		if err != nil {
			dropped = append(dropped, fmt.Sprintf("skipping the snmp probe for this cycle: %v", err))
		} else {
			opts.SNMP = &snmpprobe.Options{
				Port:        p.SNMP.Port,
				Timeout:     p.SNMP.Timeout,
				Retries:     p.SNMP.Retries,
				Credentials: creds,
			}
		}
	}

	return opts, dropped
}

// resolveSNMPCredentials maps a range's credential ids to credentials, keeping
// the configured order so the most likely credential is tried first.
func resolveSNMPCredentials(store credentialStore, ids []string) ([]snmpprobe.Credential, error) {
	if len(ids) == 0 {
		return nil, errors.New("the range references no credentials")
	}

	available, err := store.Load()
	if err != nil {
		return nil, err
	}

	creds := make([]snmpprobe.Credential, 0, len(ids))
	for _, id := range ids {
		c, ok := available[id]
		if !ok {
			return nil, fmt.Errorf("credential %q is not available on this agent", id)
		}
		if err := credentials.Validate(c); err != nil {
			return nil, err
		}
		creds = append(creds, snmpprobe.Credential{
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
