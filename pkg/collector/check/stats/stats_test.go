// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Mock Check implementation used for testing
type mockCheck struct {
	cfgSource string
	id        checkid.ID
	stringVal string
	version   string
	interval  time.Duration
}

// Mock Check interface implementation
func (mc *mockCheck) ConfigSource() string    { return mc.cfgSource }
func (mc *mockCheck) ID() checkid.ID          { return mc.id }
func (mc *mockCheck) String() string          { return mc.stringVal }
func (mc *mockCheck) Version() string         { return mc.version }
func (mc *mockCheck) Interval() time.Duration { return mc.interval }

func newMockCheck() StatsCheck {
	return &mockCheck{
		cfgSource: "checkConfigSrc",
		id:        "checkID",
		stringVal: "checkString",
		version:   "checkVersion",
		interval:  15 * time.Second,
	}
}

func newMockCheckWithInterval(interval time.Duration) StatsCheck {
	return &mockCheck{
		cfgSource: "checkConfigSrc",
		id:        "checkID",
		stringVal: "checkString",
		version:   "checkVersion",
		interval:  interval,
	}
}

func getTelemetryData() (string, error) {
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		return "", err
	}

	rec := httptest.NewRecorder()
	telemetryimpl.GetCompatComponent().Handler().ServeHTTP(rec, req)

	return rec.Body.String(), nil
}

func TestNewStats(t *testing.T) {
	stats := NewStats(newMockCheck())

	assert.Equal(t, stats.CheckID, checkid.ID("checkID"))
	assert.Equal(t, stats.CheckName, "checkString")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckConfigSource, "checkConfigSrc")
	assert.Equal(t, stats.Interval, 15*time.Second)
}

func TestNewStatsStateTelemetryInitialized(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("telemetry.checks", "*")

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
