// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the DogStatsD client telemetry component.
package mock

import (
	"testing"

	dogstatsdclienttelemetry "github.com/DataDog/datadog-agent/comp/aggregator/dogstatsdclienttelemetry/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

type mock struct{}

var _ dogstatsdclienttelemetry.Component = (*mock)(nil)

// Mock returns a no-op mock for the DogStatsD client telemetry component.
func Mock(*testing.T) dogstatsdclienttelemetry.Component {
	return &mock{}
}

func (*mock) ObserveFinalDogStatsDSerie(*metrics.Serie) {}
