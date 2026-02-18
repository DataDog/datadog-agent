// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pg_verification

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/pgsyntax"
	pgquery "github.com/pganalyze/pg_query_go/v6"
)

var (
	forbiddenFuncs  = buildFunctionMap()
	forbiddenTables = buildTableMap()
)

// VerifyForbiddenPgExpressionsAST validates using the AST parser
func VerifyForbiddenPgExpressionsAST(parsed *pgquery.ParseResult) error {

	var firstError error
	for _, stmt := range parsed.Stmts {
		pgsyntax.Traverse(stmt.Stmt, func(node *pgquery.Node) bool {
			if err := checkForbiddenFunction(node, forbiddenFuncs); err != nil {
				if firstError == nil {
					firstError = err
				}
				return false // stop early
			}

			if err := checkForbiddenTable(node, forbiddenTables); err != nil {
				if firstError == nil {
					firstError = err
				}
				return false // stop early
			}

			return true // Continue
		})

		if firstError != nil {
			return firstError
		}
	}

	return nil
}

// checkForbiddenFunction checks if a node is a forbidden function call
func checkForbiddenFunction(node *pgquery.Node, forbiddenFuncs map[string]bool) error {
	funcCall := node.GetFuncCall()
	if funcCall == nil {
		return nil
	}

	funcName := extractFunctionName(funcCall)
	if funcName == "" {
		return nil
	}

	lowerName := strings.ToLower(funcName)
	if _, forbidden := forbiddenFuncs[lowerName]; forbidden {
		return fmt.Errorf("usage of %s in a query is forbidden", lowerName)
	}

	return nil
}

// checkForbiddenTable checks if a node is a forbidden table reference
func checkForbiddenTable(node *pgquery.Node, forbiddenTables map[string]bool) error {
	rangeVar := node.GetRangeVar()
	if rangeVar == nil {
		return nil
	}

	tableName := rangeVar.Relname
	if tableName == "" {
		return nil
	}

	lowerName := strings.ToLower(tableName)
	if _, forbidden := forbiddenTables[lowerName]; forbidden {
		return fmt.Errorf("usage of %s in a query is forbidden", lowerName)
	}

	return nil
}

// extractFunctionName gets the function name from a FuncCall node
func extractFunctionName(funcCall *pgquery.FuncCall) string {
	if len(funcCall.Funcname) == 0 {
		return ""
	}

	// Function name is the last element in qualified names
	lastField := funcCall.Funcname[len(funcCall.Funcname)-1]
	if strNode := lastField.GetString_(); strNode != nil {
		return strNode.Sval
	}

	return ""
}

// buildFunctionMap creates a map of forbidden function names
func buildFunctionMap() map[string]bool {
	m := make(map[string]bool)
	for _, fn := range InfoFunctions {
		m[fn] = true
	}
	for _, fn := range AdminFunctions {
		m[fn] = true
	}
	return m
}

// buildTableMap creates a map of forbidden table names
func buildTableMap() map[string]bool {
	m := make(map[string]bool)
	for _, tbl := range Tables {
		m[tbl] = true
	}
	return m
}
