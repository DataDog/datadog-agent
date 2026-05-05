// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateMetricType(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		err := validateMetricType("gauge", "gauge")
		require.NoError(t, err)
	})

	t.Run("mismatch returns error", func(t *testing.T) {
		err := validateMetricType("gauge", "counter")
		require.ErrorContains(t, err, "does not match expected")
	})

	t.Run("case mismatch returns error", func(t *testing.T) {
		err := validateMetricType("gauge", "Gauge")
		require.ErrorContains(t, err, "does not match expected")
	})

	t.Run("missing observed type is allowed", func(t *testing.T) {
		err := validateMetricType("gauge", "")
		require.NoError(t, err)
	})
}
