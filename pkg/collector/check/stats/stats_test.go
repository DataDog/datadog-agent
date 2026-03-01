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
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Mock Check implementation used for testing
type mockCheck struct {
	cfgSource   string
	loaderName  string
	id          checkid.ID
	stringVal   string
	version     string
	interval    time.Duration
	haSupported bool
}

// Mock Check interface implementation
func (mc *mockCheck) ConfigSource() string    { return mc.cfgSource }
func (mc *mockCheck) Loader() string          { return mc.loaderName }
func (mc *mockCheck) ID() checkid.ID          { return mc.id }
func (mc *mockCheck) String() string          { return mc.stringVal }
func (mc *mockCheck) Version() string         { return mc.version }
func (mc *mockCheck) Interval() time.Duration { return mc.interval }
func (mc *mockCheck) IsHASupported() bool     { return mc.haSupported }

func newMockCheck() StatsCheck {
	return &mockCheck{
		cfgSource:   "checkConfigSrc",
		id:          "checkID",
		stringVal:   "checkString",
		loaderName:  "mockLoader",
		version:     "checkVersion",
		interval:    15 * time.Second,
		haSupported: false,
	}
}

func newMockCheckWithInterval(interval time.Duration) StatsCheck {
	return &mockCheck{
		cfgSource:   "checkConfigSrc",
		id:          "checkID",
		stringVal:   "checkString",
		loaderName:  "mockloader",
		version:     "checkVersion",
		interval:    interval,
		haSupported: false,
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
	stats := NewStats(newMockCheck(), healthplatformmock.Mock(t))

	assert.Equal(t, stats.CheckID, checkid.ID("checkID"))
	assert.Equal(t, stats.CheckName, "checkString")
	assert.Equal(t, stats.CheckLoader, "mockLoader")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckVersion, "checkVersion")
	assert.Equal(t, stats.CheckConfigSource, "checkConfigSrc")
	assert.Equal(t, stats.Interval, 15*time.Second)
	assert.Equal(t, stats.HASupported, false)
}

func TestNewStatsStateTelemetryInitialized(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("telemetry.checks", "*")

	NewStats(newMockCheck(), healthplatformmock.Mock(t))

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

func TestFormatUint64(t *testing.T) {
	assert.Equal(t, "0", formatUint64(0))
	assert.Equal(t, "1", formatUint64(1))
	assert.Equal(t, "42", formatUint64(42))
	assert.Equal(t, "18446744073709551615", formatUint64(^uint64(0))) // max uint64
}

func TestNewSenderStats(t *testing.T) {
	ss := NewSenderStats()
	assert.NotNil(t, ss.EventPlatformEvents)
	assert.Equal(t, int64(0), ss.MetricSamples)
	assert.Equal(t, int64(0), ss.Events)
	assert.Equal(t, int64(0), ss.ServiceChecks)
	assert.Equal(t, int64(0), ss.HistogramBuckets)
}

func TestSenderStatsCopy(t *testing.T) {
	ss := NewSenderStats()
	ss.MetricSamples = 10
	ss.Events = 5
	ss.EventPlatformEvents["dbm-samples"] = 42

	cp := ss.Copy()
	assert.Equal(t, int64(10), cp.MetricSamples)
	assert.Equal(t, int64(42), cp.EventPlatformEvents["dbm-samples"])

	// mutating copy doesn't affect original
	cp.EventPlatformEvents["dbm-samples"] = 99
	assert.Equal(t, int64(42), ss.EventPlatformEvents["dbm-samples"])
}

func TestStatsAdd(t *testing.T) {
	configmock.New(t)
	stats := NewStats(newMockCheck(), nil)

	senderStats := NewSenderStats()
	senderStats.MetricSamples = 10
	senderStats.Events = 3
	senderStats.ServiceChecks = 1
	senderStats.HistogramBuckets = 5
	senderStats.EventPlatformEvents["dbm-samples"] = 7

	stats.Add(100*time.Millisecond, nil, nil, senderStats, nil)

	assert.Equal(t, uint64(1), stats.TotalRuns)
	assert.Equal(t, uint64(0), stats.TotalErrors)
	assert.Equal(t, int64(10), stats.MetricSamples)
	assert.Equal(t, uint64(10), stats.TotalMetricSamples)
	assert.Equal(t, int64(3), stats.Events)
	assert.Equal(t, int64(1), stats.ServiceChecks)
	assert.Equal(t, int64(5), stats.HistogramBuckets)
	assert.Equal(t, "", stats.LastError)
	assert.NotZero(t, stats.LastSuccessDate)
	assert.Equal(t, 100*time.Millisecond, stats.LastExecutionTime)
	// Event platform events should be translated
	assert.Equal(t, int64(7), stats.TotalEventPlatformEvents["Database Monitoring Query Samples"])
}

func TestStatsAddWithError(t *testing.T) {
	configmock.New(t)
	stats := NewStats(newMockCheck(), nil)

	err := assert.AnError
	stats.Add(50*time.Millisecond, err, nil, NewSenderStats(), nil)

	assert.Equal(t, uint64(1), stats.TotalRuns)
	assert.Equal(t, uint64(1), stats.TotalErrors)
	assert.Equal(t, err.Error(), stats.LastError)
}

func TestStatsAddWithWarnings(t *testing.T) {
	configmock.New(t)
	stats := NewStats(newMockCheck(), nil)

	warnings := []error{assert.AnError, assert.AnError}
	stats.Add(50*time.Millisecond, nil, warnings, NewSenderStats(), nil)

	assert.Equal(t, uint64(2), stats.TotalWarnings)
	assert.Len(t, stats.LastWarnings, 2)
}

func TestSetStateCancelling(t *testing.T) {
	configmock.New(t)
	stats := NewStats(newMockCheck(), nil)
	assert.False(t, stats.Cancelling)
	stats.SetStateCancelling()
	assert.True(t, stats.Cancelling)
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
