// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validators

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// IsAlwaysTrue checks whether a rule always returns true
func IsAlwaysTrue(rule *eval.Rule) (bool, error) {
	// 1. Operand is * or /*, for comparison operators
	// 2. >0 for time comparison operators
	// Non sensical: process.open.name < "myfile.txt"

	parsingContext := ast.NewParsingContext()
	localModel := &model.Model{}
	if err := rule.GenEvaluator(localModel, parsingContext); err != nil {
		return false, fmt.Errorf("%w\n", err)
	}

	for _, fieldKey := range rule.GetFields() {
		fmt.Println("field:", fieldKey)
		for _, fieldValue := range rule.GetFieldValues(fieldKey) {
			if fieldValue.Type == eval.GlobValueType && fieldValue.Value == "/**" {
				return true, nil
			}

			// TODO: If regex, disallow .*
			if fieldValue.Type == eval.RegexpValueType && fieldValue.Value == ".*" {
				return true, nil
			}

			// exec.file.name == \"*\"
			if fieldValue.Type == eval.ScalarValueType && fieldValue.Value == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}
