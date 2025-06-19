// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"sort"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

const (
	// ExecutionContextTagName is the name of the execution context tag
	ExecutionContextTagName = "execution_context"
)

// RuleBucket groups rules with the same event type
type RuleBucket struct {
	rules                []*Rule
	fields               []eval.Field
	execContextRuleCount int // number of execution context rules at the start of rules
}

// AddRule adds a rule to the bucket
func (rb *RuleBucket) AddRule(rule *Rule) error {
	for _, field := range rule.GetEvaluator().GetFields() {
		index := sort.SearchStrings(rb.fields, field)
		if index < len(rb.fields) && rb.fields[index] == field {
			continue
		}
		rb.fields = append(rb.fields, "")
		copy(rb.fields[index+1:], rb.fields[index:])
		rb.fields[index] = field
	}

	if rule.Def != nil && rule.Def.Tags != nil && rule.Def.Tags[ExecutionContextTagName] == "true" {
		rb.rules = append(rb.rules, nil)
		copy(rb.rules[rb.execContextRuleCount+1:], rb.rules[rb.execContextRuleCount:])
		rb.rules[rb.execContextRuleCount] = rule
		rb.execContextRuleCount++
	} else {
		rb.rules = append(rb.rules, rule)
	}
	return nil
}

// GetRules returns the bucket rules
func (rb *RuleBucket) GetRules() []*Rule {
	return rb.rules
}
