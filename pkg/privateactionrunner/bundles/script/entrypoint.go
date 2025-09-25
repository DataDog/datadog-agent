// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package com_datadoghq_script provides script functionality for private action bundles.
package com_datadoghq_script //nolint:revive

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// Script provides script-related actions for private action bundles.
type Script struct {
}

// NewScript creates a new Script instance.
func NewScript() *Script {
	return &Script{}
}

func (s *Script) Run(ctx context.Context, actionName string, task *types.Task, credential interface{}) (any, error) {
	switch actionName {
	case "runPredefinedScript":
		return s.RunPredefinedScript(ctx, task, credential)
	default:
		return nil, fmt.Errorf("unknown action: %s", actionName)
	}
}

// GetAction returns the action with the specified name.
func (s *Script) GetAction(actionName string) types.Action {
	return s
}
