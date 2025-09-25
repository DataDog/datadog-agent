// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/postgresql/verification"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ExplainInputs struct {
	Statement string `json:"statement"`
}

type ExplainOutputs struct {
	Output any `json:"output"`
}

func (p *PostgreSQL) RunExplain(
	ctx context.Context,
	task *types.Task,
	credential interface{},
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

	var out string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, sanitizePGErrorMessage(err)
		}
		out += r
	}
	if err := rows.Err(); err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	return &ExplainOutputs{
		Output: out,
	}, nil
}

func buildExplainQueryString(ctx context.Context, statement string) (string, error) {
	if strings.HasPrefix(strings.ToLower(statement), "analyze") {
		err := errors.New("cannot include ANALYZE in statement")
		return "", utils.DefaultActionErrorWithDisplayError(err, err.Error())
	}

	// for safety reasons, we forbid access to certain admin tables and functions
	err := verification.VerifyForbiddenPgExpressions(statement)
	if err != nil {
		log.Errorf("failed due to a forbidden expression in the query %w", err)
		return "", err
	}

	return fmt.Sprintf("EXPLAIN %s", statement), nil
}
