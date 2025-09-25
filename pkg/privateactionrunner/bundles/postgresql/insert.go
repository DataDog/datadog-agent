// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/postgresql/verification"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	sq "github.com/Masterminds/squirrel"
)

type InsertInputs struct {
	Table   string   `json:"table,omitempty"`
	Columns []string `json:"columns,omitempty"`
	Values  [][]any  `json:"values,omitempty"`
}

type InsertOutputs struct {
	CommandTag any `json:"commandTag"`
}

func (p *PostgreSQL) RunInsert(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	inputs, credentialTokens, err := extractInputsAndCredentialTokens[InsertInputs](task, credential)
	if err != nil {
		return nil, err
	}

	db, err := openDB(credentialTokens)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "database", db)

	for _, values := range inputs.Values {
		queryStr, args, err := buildVerifiedInsertQueryString(ctx, inputs.Table, inputs.Columns, values)
		if err != nil {
			return nil, sanitizePGErrorMessage(err)
		}

		_, err = db.ExecContext(ctx, queryStr, args...)
		if err != nil {
			return nil, sanitizePGErrorMessage(err)
		}
	}
	return &InsertOutputs{
		CommandTag: fmt.Sprintf("INSERT 0 %v", len(inputs.Values)),
	}, nil
}

func buildVerifiedInsertQueryString(ctx context.Context, table string, columns []string, values []any) (string, []any, error) {
	queryStr, args, err := sq.Insert(table).
		Columns(columns...).
		Values(values...).
		PlaceholderFormat(sq.Dollar).
		ToSql()
	if err != nil {
		return "", nil, err
	}

	// for safety reasons, we forbid access to certain admin tables and functions
	err = verification.VerifyForbiddenPgExpressions(unsafeInterpolateQuery(queryStr, args))
	if err != nil {
		log.Errorf("failed due to a forbidden expression in the query %w", err)
		return "", nil, err
	}

	return queryStr, args, nil
}

// the unsafeInterpolateQuery() function is used exclusively to build a query string for verification purposes,
// it mustn't be used to build a query string for execution, as it may present a risk of SQL injection
func unsafeInterpolateQuery(query string, args []interface{}) string {
	for index, arg := range args {
		placeholder := fmt.Sprintf("$%d", index+1)
		argStr := fmt.Sprintf("'%v'", arg) // convert each argument to a string safely
		query = strings.Replace(query, placeholder, argStr, 1)
	}
	return query
}
