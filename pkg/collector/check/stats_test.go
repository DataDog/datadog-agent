// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	agentConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// Mock Check implementation used for testing
type mockCheck struct {
	StubCheck
	cfgSource string
	id        ID
	stringVal string
	version   string
}

// Mock Check interface implementation
func (mc *mockCheck) ConfigSource() string { return mc.cfgSource }
func (mc *mockCheck) ID() ID               { return mc.id }
func (mc *mockCheck) String() string       { return mc.stringVal }
func (mc *mockCheck) Version() string      { return mc.version }

func newMockCheck() Check {
	return &mockCheck{
		cfgSource: "checkConfigSrc",
		id:        "checkID",
		stringVal: "checkString",
		version:   "checkVersion",
	}
}

func getTelemetryData() (string, error) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return "", err
	}

	rec := httptest.NewRecorder()
	telemetry.Handler().ServeHTTP(rec, req)

	return rec.Body.String(), nil
}

func TestNewStats(t *testing.T) {
	stats := NewStats(newMockCheck())

	assert.Equal(t, stats.CheckID, ID("checkID"))
	assert.Equal(t, stats.CheckName, "checkString")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckConfigSource, "checkConfigSrc")
}

func TestNewStatsStateTelemetryIgnoredWhenGloballyDisabled(t *testing.T) {
	mockConfig := agentConfig.Mock(t)
	mockConfig.Set("telemetry.enabled", false)
	mockConfig.Set("telemetry.checks", "*")

	NewStats(newMockCheck())

	tlmData, err := getTelemetryData()
	if !assert.NoError(t, err) {
		return
	}

	// Assert that no telemetry is recorded
	assert.NotContains(t, tlmData, "checkString")
	assert.NotContains(t, tlmData, "state=\"fail\"")
	assert.NotContains(t, tlmData, "state=\"ok\"")
}

func TestNewStatsStateTelemetryInitializedWhenGloballyEnabled(t *testing.T) {
	mockConfig := agentConfig.Mock(t)
	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", "*")

	NewStats(newMockCheck())

	tlmData, err := getTelemetryData()
	if !assert.NoError(t, err) {
		return
	}

	assert.Contains(
		t,
		tlmData,
		"checks__runs{check_name=\"checkString\",state=\"fail\"} 0",
	)
	assert.Contains(
		t,
		tlmData,
		"checks__runs{check_name=\"checkString\",state=\"ok\"} 0",
	)
}

func TestTranslateEventPlatformEventTypes(t *testing.T) {
	original := map[string]interface{}{
		"EventPlatformEvents": map[string]interface{}{
			"dbm-samples":  12,
			"unknown-type": 34,
		},
		"EventPlatformEventsErrors": map[string]interface{}{
			"dbm-samples":  12,
			"unknown-type": 34,
		},
		"SomeOtherKey": map[string]interface{}{
			"dbm-samples":  12,
			"unknown-type": 34,
		},
	}
	expected := map[string]interface{}{
		"EventPlatformEvents": map[string]interface{}{
			"Database Monitoring Query Samples": 12,
			"unknown-type":                      34,
		},
		"EventPlatformEventsErrors": map[string]interface{}{
			"Database Monitoring Query Samples": 12,
			"unknown-type":                      34,
		},
		"SomeOtherKey": map[string]interface{}{
			"dbm-samples":  12,
			"unknown-type": 34,
		},
	}
	result, err := TranslateEventPlatformEventTypes(original)
	assert.NoError(t, err)
	assert.True(t, assert.ObjectsAreEqual(expected, result))
	assert.EqualValues(t, expected, result)
}
