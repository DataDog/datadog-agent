// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package versa

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// TestConfigureWarnsOnShortTimeseriesLookback runs the real Configure and captures its
// log, since the lookback warning is the only signal a too short window gives.
func TestConfigureWarnsOnShortTimeseriesLookback(t *testing.T) {
	tests := []struct {
		name     string
		instance string
		warn     bool
	}{
		{"timeseries below 30", "analytics_query_type: timeseries\nlookback_time_window_minutes: 15\n", true},
		{"timeseries at 30", "analytics_query_type: timeseries\nlookback_time_window_minutes: 30\n", false},
		{"timeseries above 30", "analytics_query_type: timeseries\nlookback_time_window_minutes: 45\n", false},
		{"timeseries lookback unset", "analytics_query_type: timeseries\n", false},
		{"tableData below 30", "analytics_query_type: tableData\nlookback_time_window_minutes: 15\n", false},
		{"default query type below 30", "lookback_time_window_minutes: 15\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&logs, pkglog.DebugLvl)
			require.NoError(t, err)
			pkglog.SetupLogger(logger, pkglog.DebugStr)
			t.Cleanup(func() {
				pkglog.SetupLogger(pkglog.Default(), pkglog.DebugStr)
				logger.Close()
			})

			raw := integration.Data("director_endpoint: 10.0.0.1\nanalytics_endpoint: https://10.0.0.2:8443\nusername: u\npassword: p\n" + tt.instance)
			id := checkid.BuildID(CheckName, integration.FakeConfigHash, raw, []byte(``))
			ms := mocksender.NewMockSender(t, id)

			chk := newCheck()
			require.NoError(t, chk.Configure(ms.GetSenderManager(), integration.FakeConfigHash, raw, []byte(``), "test", "provider"))
			logger.Flush()

			if tt.warn {
				assert.Contains(t, logs.String(), "lookback_time_window_minutes 15 may not span two analytics buckets")
			} else {
				assert.NotContains(t, logs.String(), "may not span two analytics buckets")
			}
		})
	}
}
