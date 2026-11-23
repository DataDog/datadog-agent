// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schemaimpl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// newTestViolationTelemetry returns a violationTelemetry reporting "test_tool" as install method
func newTestViolationTelemetry(t *testing.T) (*violationTelemetry, telemetry.Mock) {
	previousInstallMethodGetter := installMethodGetter
	t.Cleanup(func() { installMethodGetter = previousInstallMethodGetter })
	installMethodGetter = func(model.Reader) string { return "test_tool" }

	tlm := telemetrymock.New(t)
	return newViolationTelemetry(tlm), tlm
}

// gaugeSeries returns the series of the schema violations gauge, keyed by their tags joined with '|'
func gaugeSeries(t *testing.T, tlm telemetry.Mock) map[string]float64 {
	t.Helper()
	// the mock returns an error when the gauge has no series
	metrics, _ := tlm.GetGaugeMetric("config", "schema_violations")
	series := map[string]float64{}
	for _, metric := range metrics {
		tags := metric.Tags()
		key := strings.Join([]string{tags["config"], tags["setting"], tags["source"], tags["install_method"], tags["error"]}, "|")
		series[key] = metric.Value()
	}
	return series
}

func TestReport(t *testing.T) {
	vt, tlm := newTestViolationTelemetry(t)
	cfg := fakeReader{
		settings: map[string]interface{}{"api_key": 1234, "tags": []interface{}{1, 2}},
		sections: map[string]bool{"logs_config": true},
		sources:  map[string]model.Source{"api_key": model.SourceEnvVar},
	}

	vt.report(cfg, model.CoreSchema, []*model.Violation{
		{Path: "/api_key", SettingKey: "api_key", Reason: "got number instead of string"},
		{Path: "/tags/0", SettingKey: "tags", Reason: "got number instead of string"},
		{Path: "/tags/1", SettingKey: "tags", Reason: "got number instead of string"},
		{Path: "/logs_config", Reason: "contains unknown properties"},
		{Path: "/user_chosen_key", Reason: "is invalid"},
	})

	assert.Equal(t, map[string]float64{
		"datadog|api_key|environment-variable|test_tool|got number instead of string": 1,
		// violations sharing their tags are counted in the same series
		"datadog|tags|file|test_tool|got number instead of string": 2,
		// a violation on a section reports the section, without source as the section isn't a setting
		"datadog|logs_config|unknown|test_tool|contains unknown properties": 1,
		// an unknown key is chosen by the user and isn't reported
		"datadog|unknown|unknown|test_tool|is invalid": 1,
	}, gaugeSeries(t, tlm))
}

func TestReportDoesNotReportMessages(t *testing.T) {
	vt, tlm := newTestViolationTelemetry(t)
	cfg := fakeReader{settings: map[string]interface{}{"proxy_url": secretValue}}

	vt.report(cfg, model.CoreSchema, []*model.Violation{{
		Message:    "at '/proxy_url': '" + secretValue + "' does not match pattern '^https://'",
		Path:       "/proxy_url",
		SettingKey: "proxy_url",
		Reason:     "does not match pattern ^https://",
	}})

	series := gaugeSeries(t, tlm)
	require.Len(t, series, 1)
	for key := range series {
		assert.NotContains(t, key, secretValue)
	}
}

func TestReportReplacesSeriesOfItsConfig(t *testing.T) {
	vt, tlm := newTestViolationTelemetry(t)
	cfg := fakeReader{settings: map[string]interface{}{"api_key": 1234, "port": "abc"}}
	apiKeyViolation := &model.Violation{Path: "/api_key", SettingKey: "api_key", Reason: "got number instead of string"}
	portViolation := &model.Violation{Path: "/port", SettingKey: "port", Reason: "got string instead of integer"}

	vt.report(cfg, model.CoreSchema, []*model.Violation{apiKeyViolation})
	vt.report(cfg, model.SystemProbeSchema, []*model.Violation{apiKeyViolation})
	vt.report(cfg, model.CoreSchema, []*model.Violation{portViolation})
	assert.Equal(t, map[string]float64{
		"datadog|port|file|test_tool|got string instead of integer":        1,
		"system-probe|api_key|file|test_tool|got number instead of string": 1,
	}, gaugeSeries(t, tlm))

	// reporting the same violations again doesn't accumulate them
	vt.report(cfg, model.CoreSchema, []*model.Violation{portViolation})
	assert.Equal(t, 1.0, gaugeSeries(t, tlm)["datadog|port|file|test_tool|got string instead of integer"])

	// a configuration without violation removes its series only
	vt.report(cfg, model.CoreSchema, nil)
	assert.Equal(t, map[string]float64{
		"system-probe|api_key|file|test_tool|got number instead of string": 1,
	}, gaugeSeries(t, tlm))
}

func TestReportDoesNotGetInstallMethodWithoutViolation(t *testing.T) {
	vt, _ := newTestViolationTelemetry(t)
	installMethodGetter = func(model.Reader) string {
		t.Fatal("the install method shouldn't be read without violation")
		return ""
	}
	vt.report(fakeReader{}, model.CoreSchema, nil)
}

func TestClear(t *testing.T) {
	vt, tlm := newTestViolationTelemetry(t)
	cfg := fakeReader{settings: map[string]interface{}{"api_key": 1234}}
	violations := []*model.Violation{{Path: "/api_key", SettingKey: "api_key", Reason: "got number instead of string"}}
	vt.report(cfg, model.CoreSchema, violations)
	vt.report(cfg, model.SystemProbeSchema, violations)

	vt.clear(model.CoreSchema)
	assert.Equal(t, map[string]float64{
		"system-probe|api_key|file|test_tool|got number instead of string": 1,
	}, gaugeSeries(t, tlm))

	vt.clear(model.SystemProbeSchema)
	assert.Empty(t, gaugeSeries(t, tlm))

	// clearing a configuration without series is a no-op
	vt.clear(model.CoreSchema)
	assert.Empty(t, gaugeSeries(t, tlm))
}

func TestConfigTag(t *testing.T) {
	assert.Equal(t, "datadog", configTag(model.CoreSchema))
	assert.Equal(t, "system-probe", configTag(model.SystemProbeSchema))
	assert.Equal(t, "other", configTag("other"))
}

func TestKnownSection(t *testing.T) {
	cfg := fakeReader{
		settings: map[string]interface{}{"logs_config.enabled": true},
		sections: map[string]bool{"logs_config": true},
	}
	assert.Equal(t, "logs_config", knownSection(cfg, "/logs_config"))
	assert.Equal(t, "logs_config", knownSection(cfg, "/logs_config/user_chosen_key"))
	assert.Equal(t, "logs_config.enabled", knownSection(cfg, "/logs_config/enabled"))
	// the keys past an unknown one aren't reported, even when they match a known key
	assert.Equal(t, "", knownSection(cfg, "/user_chosen_key/logs_config"))
	assert.Equal(t, "", knownSection(cfg, ""))
	assert.Equal(t, "", knownSection(cfg, "not a json pointer"))
}

func TestInstallMethodGetter(t *testing.T) {
	// the install info environment variables take precedence over the install_info file
	for _, name := range []string{"DD_INSTALL_INFO_TOOL", "DD_INSTALL_INFO_TOOL_VERSION", "DD_INSTALL_INFO_INSTALLER_VERSION"} {
		if value, ok := os.LookupEnv(name); ok {
			t.Setenv(name, value) // restores the variable when the test ends
			require.NoError(t, os.Unsetenv(name))
		}
	}

	writeInstallInfo := func(t *testing.T, content string) fakeReader {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "install_info"), []byte(content), 0o600))
		return fakeReader{configFile: filepath.Join(dir, "datadog.yaml")}
	}

	t.Run("install info file", func(t *testing.T) {
		cfg := writeInstallInfo(t, "install_method:\n  tool: helm\n  tool_version: helm\n  installer_version: datadog-3.0.0\n")
		assert.Equal(t, "helm", installMethodGetter(cfg))
	})

	t.Run("install info file without tool", func(t *testing.T) {
		cfg := writeInstallInfo(t, "install_method:\n  tool: \"\"\n  tool_version: \"\"\n  installer_version: \"\"\n")
		assert.Equal(t, "unknown", installMethodGetter(cfg))
	})

	t.Run("invalid install info file", func(t *testing.T) {
		cfg := writeInstallInfo(t, "not: [valid")
		assert.Equal(t, "unknown", installMethodGetter(cfg))
	})

	t.Run("no install info file", func(t *testing.T) {
		cfg := fakeReader{configFile: filepath.Join(t.TempDir(), "datadog.yaml")}
		assert.Equal(t, "unknown", installMethodGetter(cfg))
	})

	t.Run("environment variables", func(t *testing.T) {
		t.Setenv("DD_INSTALL_INFO_TOOL", "docker")
		t.Setenv("DD_INSTALL_INFO_TOOL_VERSION", "docker")
		t.Setenv("DD_INSTALL_INFO_INSTALLER_VERSION", "docker-1.0")
		cfg := fakeReader{configFile: filepath.Join(t.TempDir(), "datadog.yaml")}
		assert.Equal(t, "docker", installMethodGetter(cfg))
	})
}
