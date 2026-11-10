// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/fx"

	observerfx "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/fx"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	recordernoop "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/fx-noop"
	reportertestbenchfx "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/fx-testbench"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggerdef "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilterdef "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmetadef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/require"
)

func TestFxApp(t *testing.T) {
	testFxApp(t, CLIParams{})
	testFxApp(t, CLIParams{ComponentSettings: observerimpl.ComponentSettings{UseMetricPipeline: true}})
}

func testFxApp(t *testing.T, params CLIParams) {
	t.Helper()
	fxutil.TestOneShot(t, func() {
		err := fxutil.OneShot(run, testbenchFXOptions(params)...)
		require.NoError(t, err)
	})
}

func runFxApp(t *testing.T, params CLIParams) {
	t.Helper()
	require.NoError(t, fxutil.OneShot(run, testbenchFXOptions(params)...))
}

func testbenchFXOptions(params CLIParams) []fx.Option {
	return []fx.Option{
		recordernoop.Module(),
		observerfx.Module(),
		reportertestbenchfx.Module(),
		fx.Supply(option.None[workloadmetadef.Component]()),
		fx.Supply(option.None[workloadfilterdef.Component]()),
		fx.Supply(option.None[taggerdef.Component]()),
		core.Bundle(),
		// Mirror main(): force scorer dry-run on so NewComponent yields the full
		// observerImpl (with DebugView) instead of the disabled stub, and
		// keep the agent-internal log tap off so it never ingests scenario data.
		fx.Decorate(func(c config.Component) config.Component {
			c.Set("anomaly_detection.anomaly_scorer.dry_run.enabled", true, pkgconfigmodel.SourceAgentRuntime)
			c.Set("anomaly_detection.logs.internal.enabled", false, pkgconfigmodel.SourceAgentRuntime)
			c.Set("anomaly_detection.metric_pipeline.enabled", params.ComponentSettings.UseMetricPipeline, pkgconfigmodel.SourceAgentRuntime)
			if len(params.AllowedTypes) > 0 {
				c.Set("anomaly_detection.metrics.allowed_types", params.AllowedTypes, pkgconfigmodel.SourceAgentRuntime)
			}
			return c
		}),
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(""),
			LogParams:    log.ForOneShot("", "off", true),
		}),
		fx.Supply(params),
	}
}

func TestValidateCLIParams(t *testing.T) {
	t.Run("retain override requires headless mode", func(t *testing.T) {
		err := validateCLIParams(CLIParams{RetainParquet: true})
		require.EqualError(t, err, "--retain-parquet requires --headless")
	})

	t.Run("retain override is accepted in headless mode", func(t *testing.T) {
		err := validateCLIParams(CLIParams{RetainParquet: true, Headless: "scenario"})
		require.NoError(t, err)
	})

	t.Run("detector output requires headless mode", func(t *testing.T) {
		err := validateCLIParams(CLIParams{IncludeDetectorAnomalies: true})
		require.EqualError(t, err, "--include-detector-anomalies requires --headless")
	})

	t.Run("detector output is accepted in headless mode", func(t *testing.T) {
		err := validateCLIParams(CLIParams{IncludeDetectorAnomalies: true, Headless: "scenario"})
		require.NoError(t, err)
	})

	t.Run("allowed_types flag parses valid comma-separated types", func(t *testing.T) {
		types, err := parseAllowedTypesFlag("Gauge, Count, Unknown")
		require.NoError(t, err)
		require.Equal(t, []string{"Gauge", "Count", "Unknown"}, types)
		require.NoError(t, validateCLIParams(CLIParams{
			AllowedTypes: types,
		}))
	})

	t.Run("allowed_types flag rejects invalid type and lists valid types", func(t *testing.T) {
		_, err := parseAllowedTypesFlag("Gauge,BadType")
		require.Error(t, err)
		require.Contains(t, err.Error(), `invalid metric type "BadType"`)
		require.Contains(t, err.Error(), "valid types: Unknown, Gauge, Rate, Count")
	})
}

func TestCLIWorkflowSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"components":{"scanmw":{"enabled":false,"min_points":42}}}`), 0600))
	for _, pipeline := range []bool{false, true} {
		for _, config := range []string{"", path} {
			settings, err := loadCLIComponentSettings(config, "scanmw", "", "", pipeline)
			require.NoError(t, err)
			require.Equal(t, pipeline, settings.UseMetricPipeline)
			require.Equal(t, config == "", settings.Enabled["scanmw"], "config retains precedence over component flags")
			if pipeline {
				require.Equal(t, "metric_pipeline", metricWorkflowName(pipeline))
			} else {
				require.Equal(t, "legacy", metricWorkflowName(pipeline))
			}
		}
	}
	_, err := loadCLIComponentSettings(filepath.Join(t.TempDir(), "missing"), "", "", "", true)
	require.Error(t, err)
}

func TestCLIWorkflowSelectionFromParamsFile(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		flag     bool
		pipeline bool
	}{
		{name: "file enables", body: `{"use_metric_pipeline":true,"components":{}}`, pipeline: true},
		{name: "file disables", body: `{"use_metric_pipeline":false,"components":{}}`},
		{name: "file omits key", body: `{"components":{}}`},
		{name: "flag enables when file omits key", body: `{"components":{}}`, flag: true, pipeline: true},
		{name: "flag enables when file disables", body: `{"use_metric_pipeline":false,"components":{}}`, flag: true, pipeline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "params.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.body), 0600))
			settings, err := loadCLIComponentSettings(path, "", "", "", tc.flag)
			require.NoError(t, err)
			require.Equal(t, tc.pipeline, settings.UseMetricPipeline)
		})
	}
}
