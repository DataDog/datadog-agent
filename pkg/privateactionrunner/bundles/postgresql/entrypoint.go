// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PostgreSQL struct {
	actions map[string]types.Action
}

func NewPostgreSQL() *PostgreSQL {
	return &PostgreSQL{
		actions: map[string]types.Action{
			"select":         NewSelectHandler(uint32(15 * 1024 * 1024)), // 15MB
			"insert":         NewInsertHandler(),
			"explain":        NewExplainHandler(),
			"explainAnalyze": NewExplainAnalyzeHandler(),
			"cancelQuery":    NewCancelQueryHandler(),
		},
	}
}

func (h *PostgreSQL) GetAction(actionName string) types.Action {
	return h.actions[actionName]
}
