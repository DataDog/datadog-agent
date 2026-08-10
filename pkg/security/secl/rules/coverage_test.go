// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func newCoveredRuleSet() *RuleSet {
	ruleOpts, evalOpts := NewBothOpts(map[eval.EventType]bool{"*": true})
	evalOpts.WithRuleCoverage(true)

	return NewRuleSet(&model.Model{}, newFakeEvent, ruleOpts, evalOpts)
}

func TestRuleSetCoverageDisabled(t *testing.T) {
	rs := newRuleSet()
	AddTestRuleExpr(t, rs, `open.file.path == "/etc/passwd"`)

	assert.Nil(t, rs.GetRuleCoverageReport())
}

func TestRuleSetCoverageReport(t *testing.T) {
	rs := newCoveredRuleSet()
	AddTestRuleExpr(t, rs,
		`open.file.path == "/etc/passwd" && (process.uid == 0 || process.gid == 0)`,
		`mkdir.file.path == "/tmp/foo"`,
	)

	report := rs.GetRuleCoverageReport()
	require.NotNil(t, report)

	// 4 paths for the open rule, 2 for the mkdir one
	assert.Equal(t, 2, report.TrackedRules)
	assert.Equal(t, 0, report.UntrackedRules)
	assert.Equal(t, 6, report.TotalPaths)
	assert.Equal(t, 0, report.CoveredPaths)
	require.Len(t, report.Rules, 2)

	// the rules are reported grouped by event type, mkdir before open
	assert.Equal(t, "mkdir", report.Rules[0].EventType)
	assert.Equal(t, "open", report.Rules[1].EventType)
	assert.Equal(t, "A && (B || C)", report.Rules[1].Coverage.Skeleton)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.SetFieldValue("open.file.path", "/etc/passwd")
	event.SetFieldValue("process.uid", 0)
	assert.True(t, rs.Evaluate(event))

	report = rs.GetRuleCoverageReport()
	assert.Equal(t, 1, report.CoveredPaths)
	assert.Equal(t, uint64(1), report.Rules[1].Coverage.Evaluations)
	assert.Equal(t, "A=true B=true", report.Rules[1].Coverage.Paths[3].String())
	assert.Equal(t, uint64(1), report.Rules[1].Coverage.Paths[3].Hits)

	// the report round trips through JSON, which is how it reaches the CLI
	encoded, err := json.Marshal(report)
	require.NoError(t, err)

	var decoded CoverageReport
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	assert.Equal(t, report.TotalPaths, decoded.TotalPaths)
	assert.Equal(t, report.CoveredPaths, decoded.CoveredPaths)
	assert.Equal(t, report.Rules[1].Coverage.Paths, decoded.Rules[1].Coverage.Paths)

	// and renders as a human readable summary
	rendered := decoded.String()
	assert.Contains(t, rendered, "Rule coverage: 1/6 paths (16.7%) over 2 rules")
	assert.Contains(t, rendered, "A && (B || C)")
	assert.Contains(t, rendered, `A = open.file.path == "/etc/passwd"`)
	assert.Contains(t, rendered, "[x]          1  A=true B=true")
	assert.Contains(t, rendered, "[ ]          0  A=false")

	rs.ResetRuleCoverage()
	assert.Equal(t, 0, rs.GetRuleCoverageReport().CoveredPaths)
}
