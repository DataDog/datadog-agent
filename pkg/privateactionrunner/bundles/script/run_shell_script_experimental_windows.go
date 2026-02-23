// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build private_runner_experimental && windows

package com_datadoghq_script

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// RunShellScriptHandler is a stub for Windows. The action relies on
// Unix-specific su(1) and /bin/sh and cannot run on Windows.
type RunShellScriptHandler struct{}

func NewRunShellScriptHandler() *RunShellScriptHandler {
	return &RunShellScriptHandler{}
}

func (h *RunShellScriptHandler) Run(
	_ctx context.Context,
	_task *types.Task,
	_credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("RunShellScript is not available on Windows")
}
