// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package com_datadoghq_script

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// TestConnectionHandler is a stub for Windows. The action relies on
// Unix-specific su(1) and user lookup and cannot run on Windows.
type TestConnectionHandler struct{}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	return nil, errors.New("Testing script connection is not available on Windows")
}
