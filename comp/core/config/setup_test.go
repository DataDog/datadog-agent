// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	schemanoopimpl "github.com/DataDog/datadog-agent/comp/core/schema/noop-impl"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func setupConfigWithoutFileForTest(t *testing.T, options ...func(*Params)) (pkgconfigmodel.Reader, error) {
	t.Helper()
	config := mock.New(t)
	err := setupConfig(config, &secretnooptypes.SecretNoop{}, &delegatedauthnooptypes.DelegatedAuthNoop{}, &schemanoopimpl.SchemaNoop{}, NewParams("", options...))
	return config, err
}

func TestSetupConfigIgnoreMissingConfigFileError(t *testing.T) {
	t.Setenv("DD_API_KEY", "abcdef0123456789\n")

	config, err := setupConfigWithoutFileForTest(t, WithIgnoreMissingConfigFileError(true))

	require.NoError(t, err)
	assert.Equal(t, "abcdef0123456789", config.GetString("api_key"), "post-load steps must run")
}

func TestSetupConfigIgnoreMissingConfigFileErrorReturnsPostLoadError(t *testing.T) {
	t.Setenv("DD_LOGS_CONFIG_USE_PODMAN_LOGS", "true")
	t.Setenv("DD_LOGS_CONFIG_DOCKER_PATH_OVERRIDE", "/custom/docker")

	_, err := setupConfigWithoutFileForTest(t, WithIgnoreMissingConfigFileError(true))

	assert.ErrorContains(t, err, "use_podman_logs")
}

func TestSetupConfigDefaultSkipsPostLoadOnMissingFile(t *testing.T) {
	t.Setenv("DD_API_KEY", "abcdef0123456789\n")
	t.Setenv("DD_LOGS_CONFIG_USE_PODMAN_LOGS", "true")
	t.Setenv("DD_LOGS_CONFIG_DOCKER_PATH_OVERRIDE", "/custom/docker")

	config, err := setupConfigWithoutFileForTest(t)

	require.NoError(t, err)
	assert.Equal(t, "abcdef0123456789\n", config.GetString("api_key"), "post-load steps must not run")
}
