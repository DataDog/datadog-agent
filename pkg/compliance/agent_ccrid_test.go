// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package compliance

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/compliance/metrics"
	"github.com/DataDog/datadog-agent/pkg/compliance/types"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// gaugeRecorder records the tags of the gauges sent through the statsd client.
type gaugeRecorder struct {
	statsd.ClientInterface
	tags [][]string
}

func (g *gaugeRecorder) Gauge(name string, _ float64, tags []string, _ float64) error {
	if name == metrics.MetricChecksStatuses {
		g.tags = append(g.tags, tags)
	}
	return nil
}

// TestReportedEventsCarryCCRID pins the invariant that every payload about the
// host the compliance agent reports is stamped with the host CCRID resolved at
// startup, whatever the evaluator that produced it, and that payloads about
// containers, databases and clusters are not.
func TestReportedEventsCarryCCRID(t *testing.T) {
	const hostCCRID = "arn:aws:ec2:us-east-1:123456789012:instance/i-0123456789abcdef0"

	logChan := make(chan *message.Message, 100)
	reporter := &LogReporter{
		hostname:  "test-host",
		logSource: sources.NewLogSource("test", &logsconfig.LogsConfig{Type: "test", Source: "test"}),
		logChan:   logChan,
		endpoints: &logsconfig.Endpoints{},
	}

	rule := &Rule{ID: "rule-1"}
	benchmark := &Benchmark{FrameworkID: "framework-1"}

	statsdClient := &gaugeRecorder{}
	a := &Agent{
		opts: AgentOptions{
			ResolverOptions: ResolverOptions{StatsdClient: statsdClient},
			Reporter:        reporter,
			HostCCRID:       hostCCRID,
		},
		statuses: map[string]*CheckStatus{rule.ID: {RuleID: rule.ID}},
	}

	events := []*CheckEvent{
		NewCheckEvent(RegoEvaluator, CheckPassed, nil, "host", "host", rule, benchmark),
		NewCheckEvent(XCCDFEvaluator, CheckFailed, nil, "host", "host", rule, benchmark),
		NewCheckError(XCCDFEvaluator, errors.New("boom"), "host", "host", rule, benchmark),
		// Skipped events are not reported, but are published in the statuses.
		NewCheckSkipped(RegoEvaluator, errors.New("skip"), "host", "host", rule, benchmark),
		// The host CCRID does not identify these resources: the field stays empty.
		NewCheckEvent(RegoEvaluator, CheckPassed, nil, "ctr", "docker_container", rule, benchmark),
		NewCheckEvent(RegoEvaluator, CheckFailed, nil, "pg", "db_postgresql", rule, benchmark),
		NewCheckEvent(RegoEvaluator, CheckFailed, nil, "cluster", "kubernetes_cluster", rule, benchmark),
	}
	a.reportCheckEvents(time.Minute, events...)
	require.Len(t, statsdClient.tags, len(events))
	for i, event := range events {
		if event.ResourceType == "host" {
			assert.Equal(t, hostCCRID, event.CCRID, event.ResourceType)
			assert.Contains(t, statsdClient.tags[i], "ccrid:"+hostCCRID)
		} else {
			assert.Empty(t, event.CCRID, event.ResourceType)
			for _, tag := range statsdClient.tags[i] {
				assert.NotContains(t, tag, "ccrid:", event.ResourceType)
			}
		}
	}

	a.reportResourceLog(time.Minute, NewResourceLog("host", types.ResourceType("test"), nil))
	a.reportResourceLog(time.Minute, NewResourceLog("pg", types.ResourceTypeDbPostgresql, nil))

	close(logChan)
	reported := 0
	for msg := range logChan {
		var payload struct {
			CCRID        string `json:"ccrid"`
			ResourceType string `json:"resource_type"`
		}
		require.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
		if payload.ResourceType == "host" || payload.ResourceType == "test" {
			assert.Equal(t, hostCCRID, payload.CCRID, payload.ResourceType)
		} else {
			assert.Empty(t, payload.CCRID, payload.ResourceType)
		}
		reported++
	}
	// 6 check events (the skipped one is not reported) + 2 resource logs.
	assert.Equal(t, 8, reported)
}
