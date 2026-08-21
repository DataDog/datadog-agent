// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type PostgreSQL struct {
	actions map[string]types.Action
}

func NewPostgreSQL() types.Bundle {
	return &PostgreSQL{
		actions: map[string]types.Action{
			"testConnection": NewTestConnectionHandler(),
		},
	}
}

func (p PostgreSQL) GetAction(actionName string) types.Action {
	return p.actions[actionName]
}
