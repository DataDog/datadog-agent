// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validators holds validators related files
package validators

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// HasBareWildcardInField checks whether a rule has a bare wildcard
func HasBareWildcardInField(rule *eval.Rule) (bool, error) {
	parsingContext := ast.NewParsingContext(false)
	localModel := &model.Model{}
	if err := rule.GenEvaluator(localModel, parsingContext); err != nil {
		return false, err
	}

	for _, fieldKey := range rule.GetFields() {
		for _, fieldValue := range rule.GetFieldValues(fieldKey) {
			if fieldValue.Type == eval.GlobValueType && fieldValue.Value == "/**" {
				return true, nil
			} else if fieldValue.Type == eval.RegexpValueType && fieldValue.Value == ".*" {
				// Example: dns.question.name =~ r".*"
				// matches any character (except for line terminators) >= 0 times
				return true, nil
			} else if fieldValue.Type == eval.ScalarValueType && fieldValue.Value == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}
