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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logsconfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/compliance/types"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestReportedEventsCarryHostCCRID pins the invariant that every payload the
// compliance agent reports is stamped with the host CCRID resolved at startup,
// whatever the evaluator that produced it.
func TestReportedEventsCarryHostCCRID(t *testing.T) {
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

	a := &Agent{
		opts: AgentOptions{
			Reporter:  reporter,
			HostCCRID: hostCCRID,
		},
		statuses: map[string]*CheckStatus{rule.ID: {RuleID: rule.ID}},
	}

	events := []*CheckEvent{
		NewCheckEvent(RegoEvaluator, CheckPassed, nil, "host", "host", rule, benchmark),
		NewCheckEvent(XCCDFEvaluator, CheckFailed, nil, "host", "host", rule, benchmark),
		NewCheckError(XCCDFEvaluator, errors.New("boom"), "host", "host", rule, benchmark),
		// Skipped events are not reported, but are published in the statuses.
		NewCheckSkipped(RegoEvaluator, errors.New("skip"), "host", "host", rule, benchmark),
	}
	a.reportCheckEvents(time.Minute, events...)
	for _, event := range events {
		assert.Equal(t, hostCCRID, event.HostCCRID)
	}

	a.reportResourceLog(time.Minute, NewResourceLog("host", types.ResourceType("test"), nil))

	// Everything that made it to the backend pipeline carries the field.
	close(logChan)
	reported := 0
	for msg := range logChan {
		var payload struct {
			HostCCRID string `json:"host_ccrid"`
		}
		require.NoError(t, json.Unmarshal(msg.GetContent(), &payload))
		assert.Equal(t, hostCCRID, payload.HostCCRID)
		reported++
	}
	// 3 check events (the skipped one is not reported) + 1 resource log.
	assert.Equal(t, 4, reported)
}
