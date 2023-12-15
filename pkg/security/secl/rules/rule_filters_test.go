// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package rules holds rules related files
package rules

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRuleIDFilter(t *testing.T) {
	testPolicy := &PolicyDef{
		Rules: []*RuleDefinition{
			{
				ID:         "test1",
				Expression: `open.file.path == "/tmp/test"`,
			},
			{
				ID:         "test2",
				Expression: `open.file.path != "/tmp/test"`,
			},
		},
	}

	policyOpts := PolicyLoaderOpts{
		RuleFilters: []RuleFilter{
			&RuleIDFilter{
				ID: "test2",
			},
		},
	}

	es, err := loadPolicyIntoProbeEvaluationRuleSet(t, testPolicy, policyOpts)
	rs := es.RuleSets[DefaultRuleSetTagValue]
	assert.Nil(t, err)

	assert.NotContains(t, rs.rules, "test1")
	assert.Contains(t, rs.rules, "test2")
}
