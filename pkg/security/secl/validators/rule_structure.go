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
	parsingContext := ast.NewParsingContext()
	localModel := &model.Model{}
	if err := rule.GenEvaluator(localModel, parsingContext); err != nil {
		return false, fmt.Errorf("%w\n", err)
	}

	operatorsOnSubexpressions := getOperatorsOnSubexpressions(rule.GetAst())

	var subexpressionTruthValues []bool

	for _, fieldKey := range rule.GetFields() {
		for _, fieldValue := range rule.GetFieldValues(fieldKey) {
			if fieldValue.Type == eval.GlobValueType && fieldValue.Value == "/**" {
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else if fieldValue.Type == eval.RegexpValueType && fieldValue.Value == ".*" {
				// Example: dns.question.name =~ r".*"
				// matches any character (except for line terminators) >= 0 times
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else if fieldValue.Type == eval.ScalarValueType && fieldValue.Value == "*" {
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else {
				subexpressionTruthValues = append(subexpressionTruthValues, false)
			}
		}
	}

	if len(operatorsOnSubexpressions) == 0 {
		for _, truthValue := range subexpressionTruthValues {
			if truthValue == true {
				return true, nil
			}
		}
	}

	var totalTruthValue bool

	for idx, truthValue := range subexpressionTruthValues {
		if idx == 0 {
			totalTruthValue = truthValue
		} else {
			if operatorsOnSubexpressions[idx-1] == "&&" {
				totalTruthValue = totalTruthValue && truthValue
			} else if operatorsOnSubexpressions[idx-1] == "||" {
				totalTruthValue = totalTruthValue || truthValue
			}
		}
	}

	return totalTruthValue, nil
}

func getOperatorsOnSubexpressions(st *ast.Rule) []string {
	var operators []string

	expression := st.BooleanExpression.Expression
	if expression.Op != nil {
		operators = append(operators, *expression.Op)
	}

	for expression.Next != nil {
		expression = expression.Next.Expression
		if expression.Op != nil {
			operators = append(operators, *expression.Op)
		}
	}

	return operators
}
