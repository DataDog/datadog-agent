// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pgquery "github.com/pganalyze/pg_query_go/v6"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	verification "github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/postgresql/verification"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type ExplainHandler struct{}

func NewExplainHandler() *ExplainHandler {
	return &ExplainHandler{}
}

type ExplainInputs struct {
	Statement string `json:"statement"`
}

type ExplainOutputs struct {
	Output any `json:"output"`
}

func (h *ExplainHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, credentialTokens, err := extractInputsAndCredentialTokens[ExplainInputs](task, credential)
	if err != nil {
		return nil, err
	}

	db, err := openDB(credentialTokens)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "database", db)

	statement, err := buildExplainQueryString(ctx, inputs.Statement)
	if err != nil {
		return nil, err
	}

	preparedStatement, err := db.PrepareContext(ctx, statement)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "statement", preparedStatement)

	rows, err := preparedStatement.QueryContext(ctx)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "rows", rows)

	var out strings.Builder
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, sanitizePGErrorMessage(err)
		}
		out.WriteString(r)
	}
	if err := rows.Err(); err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	return &ExplainOutputs{
		Output: out.String(),
	}, nil
}

func buildExplainQueryString(ctx context.Context, statement string) (string, error) {
	statement = "EXPLAIN " + statement
	parsed, err := pgquery.Parse(statement)
	if err != nil {
		log.FromContext(ctx).Debug("failed to parse SQL statement", log.ErrorField(err))
		return "", fmt.Errorf("invalid SQL syntax: %w", err)
	}

	// Ensure a single statement
	if len(parsed.Stmts) != 1 {
		err := errors.New("only single statements are supported")
		return "", util.DefaultActionErrorWithDisplayError(err, err.Error())
	}

	// Check if it has ANALYZE option (blacklist)
	if explainStmt := getExplainStatement(parsed); explainStmt != nil {
		if hasAnalyzeOption(explainStmt) {
			err := errors.New("statement cannot include ANALYZE or ANALYSE keyword")
			return "", util.DefaultActionErrorWithDisplayError(err, err.Error())
		}
	} else {
		err := errors.New("only 'explainable' statements are allowed")
		return "", util.DefaultActionErrorWithDisplayError(err, err.Error())
	}

	// for safety reasons, we forbid access to certain admin tables and functions
	err = verification.VerifyForbiddenPgExpressionsAST(parsed)
	if err != nil {
		log.FromContext(ctx).Error("failed due to a forbidden expression in the query", log.ErrorField(err))
		return "", err
	}

	return statement, nil
}

// getExplainStatement returns the EXPLAIN statement if present, nil otherwise
func getExplainStatement(parsed *pgquery.ParseResult) *pgquery.ExplainStmt {
	for _, stmt := range parsed.Stmts {
		if explainStmt := stmt.Stmt.GetExplainStmt(); explainStmt != nil {
			return explainStmt
		}
	}
	return nil
}

// hasAnalyzeOption checks if an EXPLAIN statement has the ANALYZE option using the AST
func hasAnalyzeOption(explainStmt *pgquery.ExplainStmt) bool {
	for _, option := range explainStmt.Options {
		if defElem := option.GetDefElem(); defElem != nil {
			// Check for both "analyze" and "analyse" (British spelling)
			if defElem.Defname == "analyze" || defElem.Defname == "analyse" {
				return true
			}
		}
	}
	return false
}
