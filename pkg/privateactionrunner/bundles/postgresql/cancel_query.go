// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	sq "github.com/Masterminds/squirrel"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type CancelQueryInputs struct {
	PID          int    `json:"pid,omitempty"`
	DatabaseName string `json:"databaseName,omitempty"`
	QueryStart   string `json:"queryStart,omitempty"`
}

type CancelQueryOutputs struct {
	Cancelled bool `json:"cancelled"`
}

func (p *PostgreSQL) RunCancelQuery(
	ctx context.Context,
	task *types.Task,
	credential interface{},
) (interface{}, error) {
	inputs, credentialTokens, err := extractInputsAndCredentialTokens[CancelQueryInputs](task, credential)
	if err != nil {
		return nil, err
	}

	db, err := openDB(credentialTokens)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "database", db)

	// build cancel query with validation
	query := sq.Select("pg_cancel_backend($1)").
		From("pg_stat_activity").
		Where("state = 'active' AND pid = $2 AND datname = $3 AND query_start = $4")
	statement, _, err := query.PlaceholderFormat(sq.Dollar).ToSql()
	if err != nil {
		return nil, err
	}

	preparedStatement, err := db.PrepareContext(ctx, statement)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	defer closeSafely(ctx, "statement", preparedStatement)

	pid := strconv.Itoa(inputs.PID)
	rows, err := preparedStatement.QueryContext(ctx, pid, pid, inputs.DatabaseName, inputs.QueryStart)
	if err != nil {
		return nil, sanitizePGErrorMessage(err)
	}

	var queryResults []bool
	for rows.Next() {
		var cancelled bool
		err = rows.Scan(&cancelled)
		if err != nil {
			return nil, sanitizePGErrorMessage(err)
		}
		queryResults = append(queryResults, cancelled)
	}
	if len(queryResults) > 1 {
		log.Warn("multiple cancelled queries found")
	}

	cancelled := len(queryResults) == 1 && queryResults[0]

	if err = rows.Err(); err != nil {
		return nil, sanitizePGErrorMessage(err)
	}
	return &CancelQueryOutputs{
		Cancelled: cancelled,
	}, nil
}
