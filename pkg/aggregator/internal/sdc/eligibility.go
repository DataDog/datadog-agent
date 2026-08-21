// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sdc

import (
	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// sdcCompressedCheckNames returns the set of check names that should get
// SDC-compressed metrics, from the checks.sdc_compression_checks config
// setting. Read fresh on every call; callers that cache eligibility for a
// check's lifetime (e.g. CheckSampler, once at creation) intentionally don't
// pick up config changes afterward, since none of these keys have
// hot-reload wiring.
func sdcCompressedCheckNames() map[string]bool {
	names := setup.Datadog().GetStringSlice("checks.sdc_compression_checks")
	m := make(map[string]bool, len(names))
	for _, name := range names {
		m[name] = true
	}
	return m
}

// EnabledFor reports whether checkName should get SDC-compressed Gauge
// metrics, per checks.sdc_compression_all/checks.sdc_compression_checks.
func EnabledFor(checkName string) bool {
	if setup.Datadog().GetBool("checks.sdc_compression_all") {
		return true
	}
	return sdcCompressedCheckNames()[checkName]
}

// CompressorConfig returns the SDC compressor tuning parameters from the
// checks.sdc_compression_* config settings (not per-metric — shared by every
// compressed context). Setting checks.sdc_compression_floor to 0 disables
// the floor entirely: Epsilon*scale (however small) always wins over a 0
// Floor, since both factors are non-negative.
func CompressorConfig() Config {
	cfg := setup.Datadog()
	return Config{
		Epsilon: cfg.GetFloat64("checks.sdc_compression_epsilon"),
		Alpha:   cfg.GetFloat64("checks.sdc_compression_alpha"),
		Floor:   cfg.GetFloat64("checks.sdc_compression_floor"),
		Warmup:  cfg.GetInt("checks.sdc_compression_warmup"),
	}
}

// DryRun reports whether checks.sdc_compression_dry_run is set: every
// sample still runs through the compressor for measurement, but nothing the
// compressor decides gets applied — every point ships unmodified.
func DryRun() bool {
	return setup.Datadog().GetBool("checks.sdc_compression_dry_run")
}

// MaxSilentFlushes returns checks.sdc_compression_max_silent_flushes: how
// many consecutive aggregator flush cycles a compressed context can have a
// pending sample with no natural breakpoint before it force-ships a point
// anyway (so a flat signal doesn't go silent forever). Counted in flush
// cycles rather than check-run commits so the same value means the same
// real-world duration for every check regardless of its own
// min_collection_interval. 0 or negative disables the heartbeat entirely
// (pure compression).
func MaxSilentFlushes() int {
	return setup.Datadog().GetInt("checks.sdc_compression_max_silent_flushes")
}
