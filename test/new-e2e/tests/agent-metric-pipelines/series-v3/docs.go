// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seriesv3 contains e2e tests that verify the use_v3_api.series routing
// configuration introduced in PR #52059.
//
// Three scenarios are covered:
//   - default (v3 on): metrics route to /api/intake/metrics/v3/series
//   - use_v3_api.series.enabled=false: metrics route to /api/v2/series
//   - use_v3_api.series.enabled=datadog_only: fakeintake is not a datadoghq.com
//     host, so the agent falls back to /api/v2/series
package seriesv3
