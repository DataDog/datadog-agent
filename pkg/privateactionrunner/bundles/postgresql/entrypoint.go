// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PostgreSQL struct {
}

func NewPostgreSQL() *PostgreSQL {
	return &PostgreSQL{}
}

func (p *PostgreSQL) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "select":
		return p.RunSelect(ctx, task, credential)
	case "insert":
		return p.RunInsert(ctx, task, credential)
	case "explain":
		return p.RunExplain(ctx, task, credential)
	case "explainAnalyze":
		return p.RunExplainAnalyze(ctx, task, credential)
	case "cancelQuery":
		return p.RunCancelQuery(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

func (p *PostgreSQL) GetAction(actionName string) types.Action {
	return p
}
