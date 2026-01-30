// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"errors"
	"strings"

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
	normalized := normalizeSQL(statement)
	normalizedLower := strings.ToLower(normalized)

	// Check for ANALYZE/ANALYSE keywords (PostgreSQL accepts both spellings)
	if strings.Contains(normalizedLower, "analyze") || strings.Contains(normalizedLower, "analyse") {
		// Further validate it's actually the ANALYZE/ANALYSE keyword
		words := strings.Fields(normalizedLower)
		for _, word := range words {
			cleanWord := strings.Trim(word, "();,")
			if cleanWord == "analyze" || cleanWord == "analyse" {
				err := errors.New("statement cannot include ANALYZE or ANALYSE keyword")
				return "", util.DefaultActionErrorWithDisplayError(err, err.Error())
			}
		}
	}

	// for safety reasons, we forbid access to certain admin tables and functions
	err := verification.VerifyForbiddenPgExpressions(statement)
	if err != nil {
		log.FromContext(ctx).Error("failed due to a forbidden expression in the query", log.ErrorField(err))
		return "", err
	}

	return "EXPLAIN " + statement, nil
}
