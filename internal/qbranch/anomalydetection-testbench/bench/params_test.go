// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
)

func TestLoadComparisonProfiles(t *testing.T) {
	profiles := []string{
		"baseline1-default.json",
		"baseline2-quantity-gauge-scanmw.json",
		"poc-quantity-gauge-scanmw-5pct.json",
	}

	for _, name := range profiles {
		t.Run(name, func(t *testing.T) {
			settings, err := LoadTestbenchParams(filepath.Join("..", "configs", name))
			require.NoError(t, err)
			settings = observerimpl.ApplyTestbenchDefaults(settings)
			require.True(t, settings.Enabled["anomaly_scorer"])
		})
	}
}
