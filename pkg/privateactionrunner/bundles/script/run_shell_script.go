// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !private_runner_experimental

package com_datadoghq_script

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type RunShellScriptHandler struct{}

func NewRunShellScriptHandler() *RunShellScriptHandler {
	return &RunShellScriptHandler{}
}

func (h *RunShellScriptHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("RunShellScript is not available")
}
