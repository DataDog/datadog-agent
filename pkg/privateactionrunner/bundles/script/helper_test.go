// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package com_datadoghq_script

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

func TestBuildEnvInjectsCredentialEnvironmentVariables(t *testing.T) {
	t.Setenv("API_TOKEN", "runner-value")
	t.Setenv("ALLOWED_VARIABLE", "allowed-value")

	env := buildEnv(
		[]string{"API_TOKEN", "ALLOWED_VARIABLE"},
		[]privateconnection.PrivateCredentialsToken{
			{Name: "API_TOKEN", Value: "credential-value"},
			{Name: "DATABASE_PASSWORD", Value: "database-password"},
		},
	)

	assert.Contains(t, env, "API_TOKEN=credential-value")
	assert.NotContains(t, env, "API_TOKEN=runner-value")
	assert.Contains(t, env, "ALLOWED_VARIABLE=allowed-value")
	assert.Contains(t, env, "DATABASE_PASSWORD=database-password")
}
