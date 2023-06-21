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

	operatorsOnSubexpressions := GetOperatorsOnSubexpressions(rule.GetAst())
	fmt.Println("printing ops")
	for _, op := range operatorsOnSubexpressions {
		fmt.Println(*op)
	}

	var subexpressionTruthValues []bool

	for _, fieldKey := range rule.GetFields() {
		fmt.Println("field:", fieldKey)
		for _, fieldValue := range rule.GetFieldValues(fieldKey) {
			fmt.Println("fieldValue:", fieldValue)

			if fieldValue.Type == eval.GlobValueType && fieldValue.Value == "/**" {
				// chunks are at rule.GetAst().BooleanExpression.Expression.Op, rule.GetAst().BooleanExpression.Expression.Next.Expression.Op
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else if fieldValue.Type == eval.RegexpValueType && fieldValue.Value == ".*" {
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else if fieldValue.Type == eval.ScalarValueType && fieldValue.Value == "*" {
				// exec.file.name == \"*\"
				subexpressionTruthValues = append(subexpressionTruthValues, true)
			} else {
				subexpressionTruthValues = append(subexpressionTruthValues, false)
			}

			//if strings.Contains(fieldKey, ".file.") && (fieldValue.Value == "/" || fieldValue.Value == "/*") {
			//	return true, nil
			//}

			// TODO: Macros and Variables
		}
	}

	//&& rule.GetAst().BooleanExpression.Expression.Comparison.ArrayComparison != nil
	if len(operatorsOnSubexpressions) == 0 {
		for _, truthValue := range subexpressionTruthValues {
			if truthValue == true {
				return true, nil
			}
		}
	}

	var firstOperand bool

	for _, operator := range operatorsOnSubexpressions {
		for idx, truthValue := range subexpressionTruthValues {
			if idx == 0 {
				firstOperand = truthValue
			} else {
				if *operator == "&&" {
					firstOperand = firstOperand && truthValue
				} else if *operator == "||" {
					firstOperand = firstOperand || truthValue
				}
			}
		}
	}

	return firstOperand, nil
}

func GetOperatorsOnSubexpressions(st *ast.Rule) []*string {
	operators := []*string{}

	expression := st.BooleanExpression.Expression
	if expression.Op != nil {
		operators = append(operators, expression.Op)
	}

	for expression.Next != nil {
		expression = expression.Next.Expression
		if expression.Op != nil {
			operators = append(operators, expression.Op)
		}
	}

	return operators
}
