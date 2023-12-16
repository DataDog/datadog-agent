// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"strings"
)

type reMacro struct {
	id    string
	count int
}

func findArray(str string) (string, int) {
	var (
		bracket int
		quote   int
		i       int
		offset  int
	)

	for i < len(str) {
		switch str[i] {
		case '"':
			quote = 1 - quote
		case '[':
			bracket++
			if quote == 0 {
				offset = i
			}
		case ']':
			bracket--
			if bracket == 0 && quote == 0 {
				return str[offset : i+1], i + 1
			}
		}
		i++
	}

	return "", -1
}

func findArrays(str string) []string {
	var arrays []string

	for {
		array, i := findArray(str)
		if i < 0 {
			break
		}

		arrays = append(arrays, array)
		str = str[i:]
	}

	return arrays
}

// ReMacro re-introduce macro
func ReMacro(def *PolicyDef) {
	// re-add macro
	var (
		macros = make(map[string]reMacro)
		n      int
	)

	for _, ruleDef := range def.Rules {
		for _, array := range findArrays(ruleDef.Expression) {
			if macro, exists := macros[array]; exists {
				macro.count++
				macros[array] = macro
			} else if len(array) > 3 {
				macros[array] = reMacro{
					id:    fmt.Sprintf("REMACRO_%d", n),
					count: 1,
				}
				n++
			}
		}
	}

	for value, macro := range macros {
		if macro.count > 1 {
			for _, ruleDef := range def.Rules {
				ruleDef.Expression = strings.ReplaceAll(ruleDef.Expression, value, macro.id)
			}
			def.Macros = append(def.Macros, &MacroDefinition{
				ID:         macro.id,
				Expression: value,
			})
		}
	}
}
