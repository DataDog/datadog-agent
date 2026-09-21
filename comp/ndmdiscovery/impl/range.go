// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"errors"
	"fmt"
	"regexp"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	ndmdiscovery "github.com/DataDog/datadog-agent/comp/ndmdiscovery/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

// minIntervalSec is the shortest cycle interval a range can ask for.
const minIntervalSec = 60

// autodiscoveryIDPattern is the character set the persistent cursor cache can
// round-trip without two ids colliding on one file.
var autodiscoveryIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// rangeConfig is the validated, defaulted form of one range.
type rangeConfig struct {
	AutodiscoveryID    string
	Namespace          string
	CIDR               string
	IntervalSec        int
	IgnoredIPAddresses []string
	Tags               []string
	Probes             probeParams
}

// rangeDefaults are the agent-side defaults applied to a range.
type rangeDefaults struct {
	Namespace    string
	IntervalSec  int
	MaxAddresses int
}

// parseRange validates and defaults one range. The returned error is surfaced
// to the backend, so it must say what is wrong with the range.
func parseRange(r ndmdiscovery.Range, def rangeDefaults, ping pingprobe.Capability, logger log.Component) (rangeConfig, error) {
	if r.ID == "" {
		return rangeConfig{}, errors.New("the range id is required")
	}
	if !autodiscoveryIDPattern.MatchString(r.ID) {
		return rangeConfig{}, fmt.Errorf("the range id %q is invalid: it must hold only letters, digits, underscores, and dashes", r.ID)
	}
	if r.CIDR == "" {
		return rangeConfig{}, errors.New("cidr is required")
	}
	if len(r.Probes) == 0 {
		return rangeConfig{}, errors.New("probes must hold at least one probe")
	}
	if _, err := newChunkPlan(r.CIDR, r.IgnoredIPAddresses, def.MaxAddresses); err != nil {
		return rangeConfig{}, err
	}

	cfg := rangeConfig{
		AutodiscoveryID:    r.ID,
		Namespace:          r.Namespace,
		CIDR:               r.CIDR,
		IntervalSec:        r.IntervalSec,
		IgnoredIPAddresses: r.IgnoredIPAddresses,
		Tags:               r.Tags,
		Probes:             parseProbes(r.ID, r.Probes, ping, logger),
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
