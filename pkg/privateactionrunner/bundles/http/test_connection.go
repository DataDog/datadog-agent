// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type TestConnectionHandler struct {
	httpHandler *Handler
}

func NewTestConnectionHandler(cfg *config.Config) *TestConnectionHandler {
	return &TestConnectionHandler{
		httpHandler: NewHttpRequestAction(cfg),
	}
}

func (h *TestConnectionHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if credential == nil || credential.HttpDetails.Testing == nil {
		return nil, errors.New("connection does not have testing configuration")
	}

	if credential.HttpDetails.BaseURL == "" {
		return nil, errors.New("connection does not have a base URL")
	}

	testing := credential.HttpDetails.Testing
	baseURL, err := url.Parse(credential.HttpDetails.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	testURL, err := baseURL.Parse(testing.Path)
	if err != nil {
		return nil, fmt.Errorf("invalid testing path: %w", err)
	}

	task.Data.Attributes.Inputs = map[string]interface{}{
		"verb": testing.Verb,
		"url":  testURL.String(),
	}

	return h.httpHandler.Run(ctx, task, credential)
}
