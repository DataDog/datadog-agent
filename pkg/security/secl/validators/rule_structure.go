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
	"regexp"
)

//Implementation
//Traverse AST. Check if each node is an "always true node".
//Evaluate each node against each node at the same level of precedence
//If at anytime, the total truth value is false, return false
//If at the end the total truth value is true, return true

func evalTruthValue(ast *ast.Rule) bool {
	var totalTruthValue bool
	//if nodeAlwaysTrue() {
	//	totalTruthValue = totalTruthValue && nodeAlwaysTrue()
	//}

	leftEvaluated := evalExpression(ast.BooleanExpression.Expression)
	rightEvaluated := evalExpression(ast.BooleanExpression.Expression.Next.Expression)

	if *ast.BooleanExpression.Expression.Op == "&&" {
		totalTruthValue = leftEvaluated && rightEvaluated
	} else if *ast.BooleanExpression.Expression.Op == "||" {
		totalTruthValue = leftEvaluated || rightEvaluated
	}

	return totalTruthValue
}

func evalExpression(expression *ast.Expression) bool {
	var totalTruthValue bool

	if expression.Comparison.ArrayComparison != nil {

	} else if expression.Comparison.ScalarComparison != nil {

	} else if expression.Comparison.ArithmeticOperation != nil {

	}

	leftEvaluated := evalExpression(expression)
	rightEvaluated := evalExpression(expression.Next.Expression)

	//if *expression.Op == "&&" {
	//	totalTruthValue = leftEvaluated && rightEvaluated
	//} else if *expression.Op == "||" {
	//	totalTruthValue = leftEvaluated || rightEvaluated
	//}

	return totalTruthValue
}

func nodeAlwaysTrue() bool {

	return false
}

func IsAlwaysTrueSimple(rule *eval.Rule) (bool, error) {
	matched1, _ := regexp.MatchString(rule.GetAst().Expr, " \"/**\"")
	matched2, _ := regexp.MatchString(rule.GetAst().Expr, " \"/*\"")

	return matched1 || matched2, nil
}
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
