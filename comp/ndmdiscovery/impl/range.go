// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"fmt"
	"regexp"

	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

// Defaults applied when a range leaves a field unset.
const (
	defaultSNMPPort       = 161
	defaultSNMPTimeoutMs  = 2000
	defaultSNMPRetries    = 1
	defaultPingCount      = 1
	defaultPingIntervalMs = 1000
	defaultPingTimeoutMs  = 1000
	minIntervalSec        = 60
)

// Upper bounds on the per-probe knobs, generous enough that only a
// misconfigured range hits them.
const (
	maxSNMPTimeoutMs  = 60_000
	maxSNMPRetries    = 10
	maxPingCount      = 10
	maxPingIntervalMs = 60_000
	maxPingTimeoutMs  = 60_000
)

// autodiscoveryIDPattern is the character set the persistent cursor cache can
// round-trip: persistentcache.GetFileForKey strips every other character
// instead of hashing, so two ids could share one cursor file.
var autodiscoveryIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// rangeConfig is the validated, defaulted form of one range. A nil
// PingOptions means ping is disabled for the range.
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

// rangeDefaults are the agent-side defaults applied to a range.
type rangeDefaults struct {
	Namespace    string
	IntervalSec  int
	MaxAddresses int
}

// parseRange validates and defaults one range. The returned error is
// surfaced to the backend, so it must say what is wrong with the range.
func parseRange(r ndmdiscovery.Range, def rangeDefaults) (rangeConfig, error) {
	if r.ID == "" {
		return rangeConfig{}, errors.New("the range id is required")
	}
	if !autodiscoveryIDPattern.MatchString(r.ID) {
		return rangeConfig{}, fmt.Errorf("the range id %q is invalid: it must hold only letters, digits, underscores, and dashes", r.ID)
	}
	if r.CIDR == "" {
		return rangeConfig{}, errors.New("cidr is required")
	}
	if len(r.CredentialIDs) == 0 {
		return rangeConfig{}, errors.New("credential_ids must hold at least one credential")
	}
	if _, err := newChunkPlan(r.CIDR, r.IgnoredIPAddresses, def.MaxAddresses); err != nil {
		return rangeConfig{}, err
	}

	cfg := rangeConfig{
		AutodiscoveryID:    r.ID,
		Namespace:          r.Namespace,
		CIDR:               r.CIDR,
		CredentialIDs:      r.CredentialIDs,
		IntervalSec:        r.IntervalSec,
		IgnoredIPAddresses: r.IgnoredIPAddresses,
		Tags:               r.Tags,
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
	if r.SNMPOptions != nil {
		if r.SNMPOptions.Port != 0 {
			snmp.Port = r.SNMPOptions.Port
		}
		if r.SNMPOptions.TimeoutMs != 0 {
			snmp.TimeoutMs = r.SNMPOptions.TimeoutMs
		}
		if r.SNMPOptions.Retries != nil {
			snmp.Retries = *r.SNMPOptions.Retries
		}
	}
	if snmp.Port < 1 || snmp.Port > 65535 {
		return rangeConfig{}, fmt.Errorf("snmp_options.port %d is out of range (expected 1-65535)", snmp.Port)
	}
	if snmp.TimeoutMs < 1 || snmp.TimeoutMs > maxSNMPTimeoutMs {
		return rangeConfig{}, fmt.Errorf("snmp_options.timeout_ms %d is out of range (expected 1-%d)", snmp.TimeoutMs, maxSNMPTimeoutMs)
	}
	// An explicit retries:0 is a legitimate do-not-retry setting.
	if snmp.Retries < 0 || snmp.Retries > maxSNMPRetries {
		return rangeConfig{}, fmt.Errorf("snmp_options.retries %d is out of range (expected 0-%d)", snmp.Retries, maxSNMPRetries)
	}
	cfg.SNMPOptions = &snmp

	if r.PingOptions != nil {
		ping := connectivity.PingOptions{
			Count:      defaultPingCount,
			IntervalMs: defaultPingIntervalMs,
			TimeoutMs:  defaultPingTimeoutMs,
		}
		if r.PingOptions.Count != 0 {
			ping.Count = r.PingOptions.Count
		}
		if r.PingOptions.IntervalMs != 0 {
			ping.IntervalMs = r.PingOptions.IntervalMs
		}
		if r.PingOptions.TimeoutMs != 0 {
			ping.TimeoutMs = r.PingOptions.TimeoutMs
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
