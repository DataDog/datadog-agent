//go:build linux_bpf

package main

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// Mock the functions for testing since they're not exported
func validateRefInstruction(arg any, subprogram *ir.Subprogram) bool {
	refValue, ok := arg.(string)
	if !ok || refValue == "" {
		return false
	}
	for _, variable := range subprogram.Variables {
		if variable.IsParameter && variable.Name == refValue {
			return true
		}
	}
	return false
}

func validateAST(astData map[string]any, subprogram *ir.Subprogram) bool {
	if len(astData) == 0 {
		return false
	}
	supportedInstructions := map[string]func(any, *ir.Subprogram) bool{
		"ref": validateRefInstruction,
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

type expressionFormat struct {
	DSL  string          `json:"dsl"`
	JSON json.RawMessage `json:"json"`
}

func expressionIsSupported(msg json.RawMessage, subprogram *ir.Subprogram) bool {
	if msg == nil {
		return false
	}
	var expr expressionFormat
	if err := json.Unmarshal(msg, &expr); err != nil {
		return false
	}

	// The JSON field can be either an object or an array of objects
	// Try parsing as an object first (JSON format)
	var ast map[string]any
	if err := json.Unmarshal(expr.JSON, &ast); err == nil {
		return validateAST(ast, subprogram)
	}

	// If that fails, try parsing as an array (YAML format)
	var astArray []map[string]any
	if err := json.Unmarshal(expr.JSON, &astArray); err != nil {
		return false
	}

	// For array format, validate each instruction in the array
	for _, astItem := range astArray {
		if !validateAST(astItem, subprogram) {
			return false
		}
	}
	return true
}

func main() {
	// Create a mock subprogram with some parameters
	subprogram := &ir.Subprogram{
		Variables: []*ir.Variable{
			{Name: "s", IsParameter: true},
			{Name: "name", IsParameter: true},
		},
	}

	fmt.Println("Testing JSON Object Format (from JSON configs):")

	// Test case 1: JSON object format - valid reference
	jsonObjectFormat := json.RawMessage(`{"dsl": "s", "json": {"ref": "s"}}`)
	fmt.Printf("  Valid ref to 's' (object): %t\n", expressionIsSupported(jsonObjectFormat, subprogram))

	// Test case 2: JSON object format - invalid reference
	jsonObjectInvalid := json.RawMessage(`{"dsl": "x", "json": {"ref": "x"}}`)
	fmt.Printf("  Invalid ref to 'x' (object): %t\n", expressionIsSupported(jsonObjectInvalid, subprogram))

	fmt.Println("\nTesting JSON Array Format (from YAML configs):")

	// Test case 3: JSON array format - valid reference (like YAML produces)
	jsonArrayFormat := json.RawMessage(`{"dsl": "s", "json": [{"ref": "s"}]}`)
	fmt.Printf("  Valid ref to 's' (array): %t\n", expressionIsSupported(jsonArrayFormat, subprogram))

	// Test case 4: JSON array format - invalid reference
	jsonArrayInvalid := json.RawMessage(`{"dsl": "x", "json": [{"ref": "x"}]}`)
	fmt.Printf("  Invalid ref to 'x' (array): %t\n", expressionIsSupported(jsonArrayInvalid, subprogram))

	// Test case 5: JSON array format - multiple instructions (all valid)
	jsonArrayMultiple := json.RawMessage(`{"dsl": "s", "json": [{"ref": "s"}, {"ref": "name"}]}`)
	fmt.Printf("  Multiple valid refs (array): %t\n", expressionIsSupported(jsonArrayMultiple, subprogram))

	// Test case 6: JSON array format - multiple instructions (one invalid)
	jsonArrayMixed := json.RawMessage(`{"dsl": "s", "json": [{"ref": "s"}, {"ref": "invalid"}]}`)
	fmt.Printf("  Mixed valid/invalid refs (array): %t\n", expressionIsSupported(jsonArrayMixed, subprogram))
}
