// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"fmt"

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
	Port      int `json:"port"`
	TimeoutMs int `json:"timeout_ms"`
	Retries   int `json:"retries"`
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
		if p.SNMPOptions.Retries != 0 {
			snmp.Retries = p.SNMPOptions.Retries
		}
	}
	if snmp.Port < 1 || snmp.Port > 65535 {
		return rangeConfig{}, fmt.Errorf("snmp_options.port %d is out of range (expected 1-65535)", snmp.Port)
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
		cfg.PingOptions = &ping
	}

	return cfg, nil
}
