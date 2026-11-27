// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"fmt"
	"regexp"
	"slices"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
)

// minIntervalSec is the shortest cycle interval a range can ask for.
const minIntervalSec = 60

// autodiscoveryIDPattern is the character set the persistent cursor cache round-trips safely.
var autodiscoveryIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// rangeConfig is the validated, defaulted form of one range.
// Keep rangeConfig.equal in sync: it decides whether a running range restarts.
type rangeConfig struct {
	AutodiscoveryID    string
	Namespace          string
	NetworkAddress     string
	IntervalSec        int
	IgnoredIPAddresses []string
	Tags               []string
	Probes             probeParams
}

// equal reports whether two configs sweep the same way, so a running range can be kept.
func (c rangeConfig) equal(o rangeConfig) bool {
	return c.AutodiscoveryID == o.AutodiscoveryID &&
		c.Namespace == o.Namespace &&
		c.NetworkAddress == o.NetworkAddress &&
		c.IntervalSec == o.IntervalSec &&
		slices.Equal(c.IgnoredIPAddresses, o.IgnoredIPAddresses) &&
		slices.Equal(c.Tags, o.Tags) &&
		c.Probes.equal(o.Probes)
}

// rangeDefaults are the agent-side defaults applied to a range.
type rangeDefaults struct {
	Namespace    string
	IntervalSec  int
	MaxAddresses int
}

// parseRange validates and defaults one range. Its error is surfaced to the backend.
func parseRange(r ndmdiscovery.Range, def rangeDefaults, logger log.Component) (rangeConfig, error) {
	if r.ID == "" {
		return rangeConfig{}, errors.New("the range id is required")
	}
	if !autodiscoveryIDPattern.MatchString(r.ID) {
		return rangeConfig{}, fmt.Errorf("the range id %q is invalid: it must hold only letters, digits, underscores, and dashes", r.ID)
	}
	if r.NetworkAddress == "" {
		return rangeConfig{}, errors.New("network_address is required")
	}
	if len(r.Probes) == 0 {
		return rangeConfig{}, errors.New("probes must hold at least one probe")
	}
	if _, err := newChunkPlan(r.NetworkAddress, r.IgnoredIPAddresses, def.MaxAddresses); err != nil {
		return rangeConfig{}, err
	}

	cfg := rangeConfig{
		AutodiscoveryID:    r.ID,
		Namespace:          r.Namespace,
		NetworkAddress:     r.NetworkAddress,
		IntervalSec:        r.IntervalSec,
		IgnoredIPAddresses: r.IgnoredIPAddresses,
		Tags:               r.Tags,
		Probes:             parseProbes(r.ID, r.Probes, logger),
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

	return cfg, nil
}
