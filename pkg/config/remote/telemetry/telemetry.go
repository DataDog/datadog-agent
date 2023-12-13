// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	subsystem = "remoteconfig"
)

var (
	commonOpts = telemetry.Options{NoDoubleUnderscoreSep: true}
)

var (
	// CacheBypassRateLimit counts how many cache bypass requests trigger rate limiting.
	CacheBypassRateLimit = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_bypass_ratelimiter_skip",
		[]string{},
		"Number of Remote Configuration cache bypass requests skipped by rate limiting.",
		commonOpts,
	)

	// CacheBypassTimeout counts how many cache bypass requests timeout
	CacheBypassTimeout = telemetry.NewCounterWithOpts(
		subsystem,
		"cache_bypass_timeout",
		[]string{},
		"Number of Remote Configuration cache bypass requests that timeout.",
		commonOpts,
	)
)
