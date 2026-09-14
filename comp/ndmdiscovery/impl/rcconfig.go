// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// kindAutodiscovery is the only config kind this component claims on the
// shared NDM Remote Configuration product. Configs of any other kind belong
// to another subscriber and are ignored silently.
const kindAutodiscovery = "autodiscovery"

// Defaults applied when the Remote Configuration payload leaves a field unset.
const (
	defaultSNMPPort       = 161
	defaultSNMPTimeoutMs  = 2000
	defaultSNMPRetries    = 1
	defaultPingCount      = 1
	defaultPingIntervalMs = 1000
	defaultPingTimeoutMs  = 1000
	minIntervalSec        = 60
)

// Upper bounds on the per-probe knobs. They are deliberately generous: the
// point is not to tune the sweep but to keep one misconfigured range from
// turning a chunk into an unbounded amount of work. A chunk is 256 addresses,
// so a per-address budget above a minute already makes a single chunk longer
// than the shortest allowed cycle, and a ping count above a handful stops
// being a liveness check.
const (
	maxSNMPTimeoutMs  = 60_000
	maxSNMPRetries    = 10
	maxPingCount      = 10
	maxPingIntervalMs = 60_000
	maxPingTimeoutMs  = 60_000
)

// autodiscoveryIDPattern is the character set the persistent cursor cache can
// round-trip. persistentcache.GetFileForKey strips every character outside
// [a-zA-Z0-9_-] from the key instead of hashing it, so "range.a", "range/a"
// and "rangea" would all resolve to the same cursor file, and an ID made only
// of stripped characters would resolve to the cache directory itself. Two
// ranges sharing one cursor silently skip each other's chunks and still report
// completed, so an ID that is not already in this character set is rejected up
// front rather than sanitised.
var autodiscoveryIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// configKind reads the kind discriminator without decoding the rest of the
// payload. A payload with no kind yields an empty string.
func configKind(raw []byte) (string, error) {
	var envelope struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	return envelope.Kind, nil
}

// snmpOptionsConfig mirrors the snake_case Remote Configuration payload.
// connectivity.SNMPOptions uses camelCase tags, so it cannot be decoded
// directly.
type snmpOptionsConfig struct {
	Port      int  `json:"port"`
	TimeoutMs int  `json:"timeout_ms"`
	Retries   *int `json:"retries"`
}

// pingOptionsConfig mirrors the snake_case Remote Configuration payload.
type pingOptionsConfig struct {
	Count      int `json:"count"`
	IntervalMs int `json:"interval_ms"`
	TimeoutMs  int `json:"timeout_ms"`
}

// rangePayload is the wire shape of one autodiscovery range config.
type rangePayload struct {
	Kind               string             `json:"kind"`
	AutodiscoveryID    string             `json:"autodiscovery_id"`
	Namespace          string             `json:"namespace"`
	CIDR               string             `json:"cidr"`
	CredentialIDs      []string           `json:"credential_ids"`
	IntervalSec        int                `json:"interval_sec"`
	IgnoredIPAddresses []string           `json:"ignored_ip_addresses"`
	Tags               []string           `json:"tags"`
	SNMPOptions        *snmpOptionsConfig `json:"snmp_options"`
	PingOptions        *pingOptionsConfig `json:"ping_options"`
}

// rangeConfig is the validated, defaulted form of one autodiscovery range.
// A nil PingOptions means ping is disabled for the range.
type rangeConfig struct {
	AutodiscoveryID    string
	Namespace          string
	CIDR               string
	CredentialIDs      []string
	IntervalSec        int
	IgnoredIPAddresses []string
	Tags               []string
	SNMPOptions        *connectivity.SNMPOptions
	PingOptions        *connectivity.PingOptions
}

// rangeDefaults are the agent-side defaults applied to a range config.
type rangeDefaults struct {
	Namespace    string
	IntervalSec  int
	MaxAddresses int
}

// parseRangeConfig decodes, validates, and defaults one autodiscovery range
// config. The returned error is surfaced to the backend through
// applyStateCallback, so it must say what is wrong with the config.
func parseRangeConfig(raw []byte, def rangeDefaults) (rangeConfig, error) {
	var p rangePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return rangeConfig{}, fmt.Errorf("failed to decode the autodiscovery config: %w", err)
	}

	if p.Kind != kindAutodiscovery {
		return rangeConfig{}, fmt.Errorf("unexpected kind %q, expected %q", p.Kind, kindAutodiscovery)
	}
	if p.AutodiscoveryID == "" {
		return rangeConfig{}, errors.New("autodiscovery_id is required")
	}
	// See autodiscoveryIDPattern: the ID is the persistent cursor key, and the
	// cache sanitises rather than hashes it, so only an ID already inside this
	// character set keeps the key namespace injective.
	if !autodiscoveryIDPattern.MatchString(p.AutodiscoveryID) {
		return rangeConfig{}, fmt.Errorf("autodiscovery_id %q is invalid: it must hold only letters, digits, underscores, and dashes", p.AutodiscoveryID)
	}
	if p.CIDR == "" {
		return rangeConfig{}, errors.New("cidr is required")
	}
	if len(p.CredentialIDs) == 0 {
		return rangeConfig{}, errors.New("credential_ids must hold at least one credential")
	}

	// Validate the range now so an unusable config is rejected before it is
	// ever scheduled.
	if _, err := newChunkPlan(p.CIDR, p.IgnoredIPAddresses, def.MaxAddresses); err != nil {
		return rangeConfig{}, err
	}

	cfg := rangeConfig{
		AutodiscoveryID:    p.AutodiscoveryID,
		Namespace:          p.Namespace,
		CIDR:               p.CIDR,
		CredentialIDs:      p.CredentialIDs,
		IntervalSec:        p.IntervalSec,
		IgnoredIPAddresses: p.IgnoredIPAddresses,
		Tags:               p.Tags,
	}
	if cfg.Namespace == "" {
		cfg.Namespace = def.Namespace
	}
	if cfg.IntervalSec <= 0 {
		cfg.IntervalSec = def.IntervalSec
	}
	if cfg.IntervalSec < minIntervalSec {
		cfg.IntervalSec = minIntervalSec
	}

	snmp := connectivity.SNMPOptions{
		Port:      defaultSNMPPort,
		TimeoutMs: defaultSNMPTimeoutMs,
		Retries:   defaultSNMPRetries,
	}
	if p.SNMPOptions != nil {
		if p.SNMPOptions.Port != 0 {
			snmp.Port = p.SNMPOptions.Port
		}
		if p.SNMPOptions.TimeoutMs != 0 {
			snmp.TimeoutMs = p.SNMPOptions.TimeoutMs
		}
		if p.SNMPOptions.Retries != nil {
			snmp.Retries = *p.SNMPOptions.Retries
		}
	}
	if snmp.Port < 1 || snmp.Port > 65535 {
		return rangeConfig{}, fmt.Errorf("snmp_options.port %d is out of range (expected 1-65535)", snmp.Port)
	}
	if snmp.TimeoutMs < 1 || snmp.TimeoutMs > maxSNMPTimeoutMs {
		return rangeConfig{}, fmt.Errorf("snmp_options.timeout_ms %d is out of range (expected 1-%d)", snmp.TimeoutMs, maxSNMPTimeoutMs)
	}
	// 0 is allowed here and only here: an explicit retries:0 is a legitimate
	// do-not-retry setting.
	if snmp.Retries < 0 || snmp.Retries > maxSNMPRetries {
		return rangeConfig{}, fmt.Errorf("snmp_options.retries %d is out of range (expected 0-%d)", snmp.Retries, maxSNMPRetries)
	}
	cfg.SNMPOptions = &snmp

	if p.PingOptions != nil {
		ping := connectivity.PingOptions{
			Count:      defaultPingCount,
			IntervalMs: defaultPingIntervalMs,
			TimeoutMs:  defaultPingTimeoutMs,
		}
		if p.PingOptions.Count != 0 {
			ping.Count = p.PingOptions.Count
		}
		if p.PingOptions.IntervalMs != 0 {
			ping.IntervalMs = p.PingOptions.IntervalMs
		}
		if p.PingOptions.TimeoutMs != 0 {
			ping.TimeoutMs = p.PingOptions.TimeoutMs
		}
		if ping.Count < 1 || ping.Count > maxPingCount {
			return rangeConfig{}, fmt.Errorf("ping_options.count %d is out of range (expected 1-%d)", ping.Count, maxPingCount)
		}
		if ping.IntervalMs < 1 || ping.IntervalMs > maxPingIntervalMs {
			return rangeConfig{}, fmt.Errorf("ping_options.interval_ms %d is out of range (expected 1-%d)", ping.IntervalMs, maxPingIntervalMs)
		}
		if ping.TimeoutMs < 1 || ping.TimeoutMs > maxPingTimeoutMs {
			return rangeConfig{}, fmt.Errorf("ping_options.timeout_ms %d is out of range (expected 1-%d)", ping.TimeoutMs, maxPingTimeoutMs)
		}
		cfg.PingOptions = &ping
	}

	return cfg, nil
}
