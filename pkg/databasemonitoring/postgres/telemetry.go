// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package postgres contains Postgres specific telemetry
package postgres

import (
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	// TlmPostgresActivityLatency is the time for the activity gathering to complete
	TlmPostgresActivityLatency = telemetry.NewHistogram("postgres", "activity_latency", nil, "Histogram of activity query latency in ms", []float64{10, 25, 50, 75, 100, 250, 500, 1000, 10000})
)
