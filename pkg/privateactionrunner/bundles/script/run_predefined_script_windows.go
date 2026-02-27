// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunPredefinedScriptHandler struct {
}

func NewRunPredefinedScriptHandler() *RunPredefinedScriptHandler {
	return &RunPredefinedScriptHandler{}
}

func (h *RunPredefinedScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("RunPredefinedScript is not available on Windows. Use PowerShell action instead")
}
