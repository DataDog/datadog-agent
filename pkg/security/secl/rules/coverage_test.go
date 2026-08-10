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

	// 5 paths for the open rule, 2 for the mkdir one
	assert.Equal(t, 2, report.TrackedRules)
	assert.Equal(t, 0, report.UntrackedRules)
	assert.Equal(t, 7, report.TotalPaths)
	assert.Equal(t, 0, report.CoveredPaths)
	require.Len(t, report.Rules, 2)

	// the rules are reported grouped by event type, mkdir before open
	assert.Equal(t, "mkdir", report.Rules[0].EventType)
	assert.Equal(t, "open", report.Rules[1].EventType)

	// resolving the file path costs more than reading the credentials, so the
	// alternative is tested before the path comparison it is written after
	open := report.Rules[1].Coverage
	assert.Equal(t, "(B || C) && A", open.Skeleton)
	assert.Equal(t, `open.file.path == "/etc/passwd"`, open.Leaves[0].Expression)
	assert.Equal(t, "A", open.Leaves[0].Name)

	var paths []string
	for _, path := range open.Paths {
		paths = append(paths, path.String())
	}
	assert.Equal(t, []string{
		"B=false C=false",
		"B=false C=true A=false",
		"B=false C=true A=true",
		"B=true A=false",
		"B=true A=true",
	}, paths)

	event := model.NewFakeEvent()
	event.Type = uint32(model.FileOpenEventType)
	event.SetFieldValue("open.file.path", "/etc/passwd")
	event.SetFieldValue("process.uid", 0)
	assert.True(t, rs.Evaluate(event))

	report = rs.GetRuleCoverageReport()
	assert.Equal(t, 1, report.CoveredPaths)
	assert.Equal(t, uint64(1), report.Rules[1].Coverage.Evaluations)
	assert.Equal(t, "B=true A=true", report.Rules[1].Coverage.Paths[4].String())
	assert.Equal(t, uint64(1), report.Rules[1].Coverage.Paths[4].Hits)

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
	assert.Contains(t, rendered, "Rule coverage: 1/7 paths (14.3%) over 2 rules")
	assert.Contains(t, rendered, "(B || C) && A")
	assert.Contains(t, rendered, `A = open.file.path == "/etc/passwd"`)
	assert.Contains(t, rendered, "[x]          1  B=true A=true")
	assert.Contains(t, rendered, "[ ]          0  B=false C=false")

	rs.ResetRuleCoverage()
	assert.Equal(t, 0, rs.GetRuleCoverageReport().CoveredPaths)
}
