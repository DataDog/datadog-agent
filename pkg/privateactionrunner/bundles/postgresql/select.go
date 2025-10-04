// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"reflect"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundles/postgresql/verification"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	sq "github.com/Masterminds/squirrel"
)

type SelectHandler struct {
	MaxBytes uint32
}

func NewSelectHandler(maxBytes uint32) *SelectHandler {
	return &SelectHandler{MaxBytes: maxBytes}
}

type SelectInputs struct {
	Columns     []string `json:"columns,omitempty"`
	Table       string   `json:"table,omitempty"`
	JoinClause  string   `json:"joinClause,omitempty"`
	WhereClause string   `json:"whereClause,omitempty"`
	OrderBy     string   `json:"orderBy,omitempty"`
	Limit       uint64   `json:"limit,omitempty"`
	Parameters  []any    `json:"parameters,omitempty"`
}

type SelectOutputs struct {
	Query            string   `json:"query"`
	TruncatedResults bool     `json:"truncatedResults"`
	Results          [][]any  `json:"results"`
	Columns          []string `json:"columns"`
}

func (h *SelectHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	inputs, credentialTokens, err := extractInputsAndCredentialTokens[SelectInputs](task, credential)
	if err != nil {
		return nil, err
	}

	db, err := openDB(credentialTokens)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "database", db)

	statement, err := buildSelectQueryString(inputs)
	if err != nil {
		return nil, err
	}

	// for safety reasons, we forbid access to certain admin tables and functions
	err = verification.VerifyForbiddenPgExpressions(statement)
	if err != nil {
		log.Errorf("failed due to a forbidden expression in the query %w", err)
		return nil, err
	}

	preparedStatement, err := db.PrepareContext(ctx, statement)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "statement", preparedStatement)

	rows, err := preparedStatement.QueryContext(ctx, inputs.Parameters...)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "rows", rows)

	columns, err := rows.Columns()
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}

	var output [][]any
	const maxBytes uint32 = 15 * 1024 * 1024 // 15MB
	var size uint32
	truncatedResults := false

	rowValues := make([]reflect.Value, len(columnTypes))
	for i := 0; i < len(columnTypes); i++ {
		rowValues[i] = reflect.New(reflect.PtrTo(columnTypes[i].ScanType()))
	}

	for rows.Next() {
		rowResult := make([]interface{}, len(columnTypes))
		for i := 0; i < len(columnTypes); i++ {
			rowResult[i] = rowValues[i].Interface()
		}

		err := rows.Scan(rowResult...)
		if err != nil {
			return nil, sanitizePGErrorMessage(err)
		}

		for i := 0; i < len(rowValues); i++ {
			if rv := rowValues[i].Elem(); rv.IsNil() {
				rowResult[i] = nil
			} else {
				rowResult[i] = rv.Elem().Interface()
			}
		}

		rowSize := unsafe.Sizeof(rowResult)
		size += uint32(rowSize)
		if size > h.MaxBytes {
			log.Warn("Reached maximum output size before processing all rows.")
			truncatedResults = true
			break
		}
		output = append(output, rowResult)
	}

	if err = rows.Err(); err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	return &SelectOutputs{
		Query:            statement,
		TruncatedResults: truncatedResults,
		Results:          output,
		Columns:          columns,
	}, nil
}

func buildSelectQueryString(inputs SelectInputs) (string, error) {
	query := sq.Select(inputs.Columns...).From(inputs.Table).Where(inputs.WhereClause)

	if inputs.JoinClause != "" {
		query = query.JoinClause(inputs.JoinClause)
	}
	if inputs.OrderBy != "" {
		query = query.OrderBy(inputs.OrderBy)
	}
	if inputs.Limit != 0 {
		query = query.Limit(inputs.Limit)
	}

	queryStr, _, err := query.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return "", err
	}
	return queryStr, nil
}
