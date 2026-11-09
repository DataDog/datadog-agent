// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tmplvar

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetEnvvarRecordsAgentEnvVarResolutions(t *testing.T) {
	cfg := configmock.New(t)
	t.Setenv("DD_API_KEY", "secret")
	t.Setenv("DB_PASSWORD", "secret")

	before := func(envVar, allowed string) float64 {
		return agentEnvVarResolutions.WithValues(envVar, allowed).Get()
	}
	apiKeyAllowed := before("DD_API_KEY", "true")
	apiKeyDenied := before("DD_API_KEY", "false")
	dbPassword := before("DB_PASSWORD", "true")

	_, err := GetEnvvar("DD_API_KEY", nil)
	assert.NoError(t, err)
	_, err = GetEnvvar("DB_PASSWORD", nil)
	assert.NoError(t, err)

	cfg.SetInTest("ad_allowed_env_vars", []string{"DB_PASSWORD"})
	_, err = GetEnvvar("DD_API_KEY", nil)
	assert.Error(t, err)

	assert.Equal(t, apiKeyAllowed+1, before("DD_API_KEY", "true"))
	assert.Equal(t, apiKeyDenied+1, before("DD_API_KEY", "false"))
	assert.Equal(t, dbPassword, before("DB_PASSWORD", "true"))
}
