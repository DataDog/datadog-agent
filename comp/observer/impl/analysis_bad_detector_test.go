// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockLogView implements observer.LogView for testing.
type mockLogView struct {
	content  []byte
	status   string
	tags     []string
	hostname string
}

func (m *mockLogView) GetContent() []byte  { return m.content }
func (m *mockLogView) GetStatus() string   { return m.status }
func (m *mockLogView) GetTags() []string   { return m.tags }
func (m *mockLogView) GetHostname() string { return m.hostname }

func TestBadDetector_Name(t *testing.T) {
	d := &BadDetector{}
	assert.Equal(t, "bad_detector", d.Name())
}

func TestBadDetector_Analyze_NoMatch(t *testing.T) {
	d := &BadDetector{}
	log := &mockLogView{
		content: []byte("everything is fine"),
		tags:    []string{"env:test"},
	}

	result := d.Analyze(log)

	assert.Empty(t, result.Metrics)
	assert.Empty(t, result.Anomalies)
}

func TestBadDetector_Analyze_Match(t *testing.T) {
	d := &BadDetector{}
	log := &mockLogView{
		content: []byte("oh no this is bad news"),
		tags:    []string{"env:prod", "service:api"},
	}

	result := d.Analyze(log)

	assert.Len(t, result.Metrics, 1)
	assert.Equal(t, "observer.bad_logs.count", result.Metrics[0].Name)
	assert.Equal(t, float64(1), result.Metrics[0].Value)
	assert.Equal(t, []string{"env:prod", "service:api"}, result.Metrics[0].Tags)

	assert.Len(t, result.Anomalies, 1)
	assert.Equal(t, "Bad log detected", result.Anomalies[0].Title)
	assert.Equal(t, "oh no this is bad news", result.Anomalies[0].Description)
	assert.Equal(t, []string{"env:prod", "service:api"}, result.Anomalies[0].Tags)
}
