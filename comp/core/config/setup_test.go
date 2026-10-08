// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	delegatedauthnooptypes "github.com/DataDog/datadog-agent/comp/core/delegatedauth/noop-impl/types"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The products used here come from the real schema: 'log_management' sets 'logs_enabled', 'apm' and
// 'error_tracking_standalone' are mutually exclusive.

// setupProductTest writes datadog.yaml (and the fleet policy if not empty)
func setupProductTest(t *testing.T, datadogYAML, fleetPolicyYAML string) (pkgconfigmodel.BuildableConfig, string, string) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(datadogYAML), 0o600))

	fleetDir := ""
	if fleetPolicyYAML != "" {
		fleetDir = filepath.Join(dir, "fleet")
		require.NoError(t, os.Mkdir(fleetDir, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(fleetDir, "datadog.yaml"), []byte(fleetPolicyYAML), 0o600))
	}
	return mock.New(t), configFile, fleetDir
}

func runSetupConfig(config pkgconfigmodel.BuildableConfig, params Params) error {
	return setupConfig(config, &secretnooptypes.SecretNoop{}, &delegatedauthnooptypes.DelegatedAuthNoop{}, params)
}

func TestSetupConfigProductEnablement(t *testing.T) {
	config, configFile, _ := setupProductTest(t, "products: [log_management]", "")

	require.NoError(t, runSetupConfig(config, NewAgentParams(configFile)))
	assert.True(t, config.GetBool("logs_enabled"))
	assert.Equal(t, pkgconfigmodel.SourceProductEnablement, config.GetSource("logs_enabled"))
}

// Products are applied by LoadDatadog, before fleet policies are merged: products set through fleet policies are ignored
func TestSetupConfigProductEnablementFromFleetPolicyIgnored(t *testing.T) {
	config, configFile, fleetDir := setupProductTest(t, "{}", "products: [log_management]")

	require.NoError(t, runSetupConfig(config, NewAgentParams(configFile, WithFleetPoliciesDirPath(fleetDir))))
	assert.Equal(t, []string{"log_management"}, config.GetStringSlice("products"))
	assert.False(t, config.GetBool("logs_enabled"))
	assert.Equal(t, map[string]interface{}{}, config.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
}

func TestSetupConfigProductConflictStrict(t *testing.T) {
	config, configFile, _ := setupProductTest(t, "products: [apm, error_tracking_standalone]", "")

	err := runSetupConfig(config, NewAgentParams(configFile, WithStrictProductEnablement(), WithCLIOverride("log_level", "debug")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "'apm' and 'error_tracking_standalone' can't be enabled together")
	// the configuration is still fully loaded
	assert.Equal(t, "debug", config.GetString("log_level"))
	assert.Equal(t, map[string]interface{}{}, config.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
}

func TestSetupConfigProductConflictNotStrict(t *testing.T) {
	var logs bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndMsgFormat(&logs, log.InfoLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "info")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "info") })

	config, configFile, _ := setupProductTest(t, "products: [apm, error_tracking_standalone]", "")

	require.NoError(t, runSetupConfig(config, NewAgentParams(configFile, WithCLIOverride("log_level", "debug"))))
	assert.Equal(t, "debug", config.GetString("log_level"))
	assert.Equal(t, map[string]interface{}{}, config.AllSettingsBySource()[pkgconfigmodel.SourceProductEnablement])
	assert.Contains(t, logs.String(), "'apm' and 'error_tracking_standalone' can't be enabled together")
}
