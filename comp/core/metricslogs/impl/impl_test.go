// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metricslogsimpl

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metricslogs "github.com/DataDog/datadog-agent/comp/core/metricslogs/def"
)

// recordingLog is a minimal logcomp.Component test double that records
// every Debugf/Tracef call so tests can assert on what LogMetrics wrote.
type recordingLog struct {
	debugf []string
	tracef []string
}

func (r *recordingLog) Trace(...interface{}) {}
func (r *recordingLog) Tracef(format string, params ...interface{}) {
	r.tracef = append(r.tracef, fmt.Sprintf(format, params...))
}
func (r *recordingLog) Debug(...interface{}) {}
func (r *recordingLog) Debugf(format string, params ...interface{}) {
	r.debugf = append(r.debugf, fmt.Sprintf(format, params...))
}
func (r *recordingLog) Info(...interface{})                 {}
func (r *recordingLog) Infof(string, ...interface{})        {}
func (r *recordingLog) Warn(...interface{}) error           { return nil }
func (r *recordingLog) Warnf(string, ...interface{}) error  { return nil }
func (r *recordingLog) Error(...interface{}) error          { return nil }
func (r *recordingLog) Errorf(string, ...interface{}) error { return nil }
func (r *recordingLog) Critical(...interface{}) error       { return nil }
func (r *recordingLog) Criticalf(string, ...interface{}) error {
	return nil
}
func (r *recordingLog) Flush() {}

func TestLogMetrics_SingleCallProducesOneDebugLine(t *testing.T) {
	log := &recordingLog{}
	c := &metricsLogsComponent{log: log}

	err := c.LogMetrics([]*metricslogs.Metric{
		{Name: "m1", Type: metricslogs.MetricTypeGauge, Value: 1, Tags: []string{"a:b"}},
		{Name: "m2", Type: metricslogs.MetricTypeCount, Value: 2, Tags: []string{"c:d"}},
	})
	require.NoError(t, err)

	require.Len(t, log.debugf, 1)

	var payload struct {
		Metrics []map[string]any `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal([]byte(log.debugf[0][len("metricslogs: "):]), &payload))
	assert.Len(t, payload.Metrics, 2)
}

func TestLogMetrics_EmptyBatchIsNoop(t *testing.T) {
	log := &recordingLog{}
	c := &metricsLogsComponent{log: log}

	err := c.LogMetrics(nil)
	require.NoError(t, err)
	assert.Empty(t, log.debugf)
	assert.Empty(t, log.tracef)
}

func TestLogMetrics_DefaultLevelIsDebug(t *testing.T) {
	log := &recordingLog{}
	c := &metricsLogsComponent{log: log}

	err := c.LogMetrics([]*metricslogs.Metric{
		{Name: "m1", Type: metricslogs.MetricTypeGauge, Value: 1},
	})
	require.NoError(t, err)
	assert.Len(t, log.debugf, 1)
	assert.Empty(t, log.tracef)
}

func TestLogMetrics_WithLevelTrace(t *testing.T) {
	log := &recordingLog{}
	c := &metricsLogsComponent{log: log}

	err := c.LogMetrics([]*metricslogs.Metric{
		{Name: "m1", Type: metricslogs.MetricTypeGauge, Value: 1},
	}, metricslogs.WithLevel(metricslogs.LevelTrace))
	require.NoError(t, err)
	assert.Len(t, log.tracef, 1)
	assert.Empty(t, log.debugf)
}
