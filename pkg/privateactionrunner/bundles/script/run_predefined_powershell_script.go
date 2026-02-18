// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !(private_runner_experimental && windows)

package com_datadoghq_script

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunPredefinedPowershellScriptHandler is a stub for non-Windows platforms
// or when the private_runner_experimental build tag is not set.
type RunPredefinedPowershellScriptHandler struct{}

// NewRunPredefinedPowershellScriptHandler returns a stub handler.
func NewRunPredefinedPowershellScriptHandler() *RunPredefinedPowershellScriptHandler {
	return &RunPredefinedPowershellScriptHandler{}
}

func (h *RunPredefinedPowershellScriptHandler) Run(
	_ context.Context,
	_ *types.Task,
	_ *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("RunPredefinedPowershellScript is not available")
}
