// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

const (
	// ExecutionContextTagName is the name of the execution context tag
	ExecutionContextTagName = "execution_context"
)

// RuleBucket groups rules with the same event type
type RuleBucket struct {
	rules  []*Rule
	fields []eval.Field
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
	rb.rules = append(rb.rules, rule)

	// sort by policy, execution context, then by priority
	sort.SliceStable(rb.rules, func(i, j int) bool {
		// sort by execution context
		var execTagsI, execTagsJ string
		if rb.rules[i].Def.Tags != nil {
			execTagsI = rb.rules[i].Def.Tags[ExecutionContextTagName]
		}
		if rb.rules[j].Def.Tags != nil {
			execTagsJ = rb.rules[j].Def.Tags[ExecutionContextTagName]
		}

		if !strings.EqualFold(execTagsI, execTagsJ) {
			return strings.EqualFold(execTagsI, "true")
		}

		// sort by policy type
		if rb.rules[i].Policy.InternalType != rb.rules[j].Policy.InternalType {
			return rb.rules[i].Policy.InternalType == DefaultPolicyType
		}

		// sort by priority
		return rb.rules[i].Def.Priority > rb.rules[j].Def.Priority
	})
	return nil
}

// GetRules returns the bucket rules
func (rb *RuleBucket) GetRules() []*Rule {
	return rb.rules
}
