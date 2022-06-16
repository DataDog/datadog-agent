// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

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

type testStatsTelemetrySender struct {
	counts []string
}

func (tsp *testStatsTelemetrySender) Count(metric string, value float64, hostname string, tags []string) {
	tsp.counts = append(tsp.counts, fmt.Sprintf("%s on %s: %f %s", metric, hostname, value, strings.Join(tags, ",")))
}

func (tsp *testStatsTelemetrySender) Gauge(metric string, value float64, hostname string, tags []string) {
	// not used
}

func TestNewStatsStateTelemetryInitialized(t *testing.T) {
	sender := &testStatsTelemetrySender{}
	telemetry.RegisterStatsSender(sender)

	NewStats(newMockCheck())

	assert.Contains(t, sender.counts, "datadog.agent.checks.runs on : 0.000000 check_name:checkString,state:ok")
	assert.Contains(t, sender.counts, "datadog.agent.checks.runs on : 0.000000 check_name:checkString,state:fail")
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
