// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_datadog_agentactions

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type HelloWorldHandler struct{}

func NewHelloWorldHandler() *HelloWorldHandler {
	return &HelloWorldHandler{}
}

type HelloWorldInputs struct {
	Message string `json:"message,omitempty"`
}

type HelloWorldOutputs struct {
	Greeting string `json:"greeting"`
	Message  string `json:"message,omitempty"`
}

func (h *HelloWorldHandler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (outputs interface{}, err error) {
	inputs, err := types.ExtractInputs[HelloWorldInputs](task)
	if err != nil {
		return nil, err
	}

	message := inputs.Message
	if message == "" {
		message = "Hello World!"
	}

	return &HelloWorldOutputs{
		Greeting: "Hello from the Datadog Agent Actions!",
		Message:  message,
	}, nil
}
