// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_gitlab_users

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct {
	currentUserHandler *CurrentUserHandler
}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{
		currentUserHandler: NewCurrentUserHandler(),
	}
}

type TestConnectionInputs struct{}

type TestConnectionOutputs = CurrentUserOutputs

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (any, error) {
	return h.currentUserHandler.Run(ctx, task, credential)
}
