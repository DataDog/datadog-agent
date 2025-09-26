// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package dyninstexprlang provides utilities for working with expressions in the expression language
// of the dynamic instrumentation and live debugger products.
package dyninstexprlang

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// expressionFormat represents the structure of expression JSON messages
type expressionFormat struct {
	DSL  string          `json:"dsl"`
	JSON json.RawMessage `json:"json"`
}

// supportedInstruction represents a validation function for an AST instruction
type supportedInstruction func(arg any, subprogram *ir.Subprogram) bool

// supportedInstructions maps instruction names to their validation functions
var supportedInstructions = map[string]supportedInstruction{
	"ref": validateRefInstruction,
}

// validateRefInstruction validates the "ref" instruction argument
func validateRefInstruction(arg any, subprogram *ir.Subprogram) bool {
	// ref instruction expects a string argument
	refValue, ok := arg.(string)
	if !ok || refValue == "" {
		return false
	}
	// Check if the referenced parameter exists in the subprogram's parameters
	for _, variable := range subprogram.Variables {
		if variable.IsParameter && variable.Name == refValue {
			return true
		}
	}
	return false
}

type variableExtractor func(arg any, subprogram *ir.Subprogram) (string, bool)

var variableExtractors = map[string]variableExtractor{
	"ref": extractVariableFromRefInstruction,
}

func extractVariableFromRefInstruction(arg any, subprogram *ir.Subprogram) (string, bool) {
	refValue, ok := arg.(string)
	if !ok || refValue == "" {
		return "", false
	}
	return refValue, true
}

// validateAST validates that all instructions in the AST are supported
func validateAST(astData map[string]any, subprogram *ir.Subprogram) bool {
	if len(astData) == 0 {
		return false
	}
	for instruction, argument := range astData {
		validator, ok := supportedInstructions[instruction]
		if !ok {
			return false
		}
		if !validator(argument, subprogram) {
			return false
		}
	}
	return true
}

// ExpressionIsSupported checks if an expression is supported by the expression language
// by parsing the JSON field as a generic AST (map of instructions to arguments) and
// validating that all instructions and arguments in the AST are supported.
func ExpressionIsSupported(msg json.RawMessage, subprogram *ir.Subprogram) bool {
	if msg == nil {
		return false
	}

	// Parse the JSON field as a generic AST (map of instructions to arguments)
	var ast map[string]any
	if err := json.Unmarshal(msg, &ast); err != nil {
		return false
	}

	// Validate that all instructions in the AST are supported
	return validateAST(ast, subprogram)
}

// CollectSegmentVariables collects the variables used in an expression
func CollectSegmentVariables(msg json.RawMessage, subprogram *ir.Subprogram) []ir.Variable {
	var variables []ir.Variable
	var ast map[string]any
	if err := json.Unmarshal(msg, &ast); err != nil {
		return variables
	}

	for instruction, argument := range ast {
		extractor, ok := variableExtractors[instruction]
		if !ok {
			continue
		}
		if varName, ok := extractor(argument, subprogram); ok {
			variables = append(variables, ir.Variable{
				Name: varName,
			})
		}
	}

	return variables
}

/*
Testdata:

msg = "{"ref": "s"}"

subprogram = {
	variables = [
		{Name: "s", IsParameter: true},
	]
}

CollectSegmentVariables(msg, subprogram) should return [s]
*/
