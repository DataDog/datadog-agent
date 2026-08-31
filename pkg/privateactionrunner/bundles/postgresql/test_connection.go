// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_postgresql

import (
	"context"
	"sort"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

// redactedTokenNames lists connection tokens whose value must never be logged or
// echoed back, even in this debug-only handler.
var redactedTokenNames = map[string]struct{}{
	privateconnection.PasswordTokenName: {},
}

const redactedValue = "***REDACTED***"

type TestConnectionHandler struct{}

func NewTestConnectionHandler() *TestConnectionHandler {
	return &TestConnectionHandler{}
}

// TestConnectionOutputs echoes back every token the runner resolved for the
// connection, so a user can verify secret resolution without a real DB round trip.
type TestConnectionOutputs struct {
	Tokens map[string]string `json:"tokens"`
}

// Run does not open a database connection; it only reports what the connection's
// tokens resolved to, for debugging secret backend configuration.
func (h *TestConnectionHandler) Run(ctx context.Context, _ *types.Task, credential *privateconnection.PrivateCredentials) (interface{}, error) {
	logger := log.FromContext(ctx)
	tokens := credential.AsTokenMap()

	names := make([]string, 0, len(tokens))
	for name := range tokens {
		names = append(names, name)
	}
	sort.Strings(names)

	resolved := make(map[string]string, len(tokens))
	for _, name := range names {
		value := tokens[name]
		if _, sensitive := redactedTokenNames[name]; sensitive {
			value = redactedValue
		}
		logger.Info("resolved postgresql connection token", log.String("name", name), log.String("value", value))
		resolved[name] = value
	}

	return &TestConnectionOutputs{Tokens: resolved}, nil
}
