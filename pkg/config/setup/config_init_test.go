// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !serverless

package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Regression test: the HOST_ETC repo dir rewrite used to run before the config was ready
// (and before HOST_ETC auto-detection), so it silently never applied.
func TestPostProcessSystemProbeRunsAfterConfigIsReady(t *testing.T) {
	origDatadog := Datadog().(pkgconfigmodel.BuildableConfig)
	origSystemProbe := SystemProbe().(pkgconfigmodel.BuildableConfig)
	t.Cleanup(func() {
		SetDatadog(origDatadog)         // nolint: forbidigo // restoring the singleton after the test
		SetSystemProbe(origSystemProbe) // nolint: forbidigo // restoring the singleton after the test
	})

	InitConfigObjects()

	t.Setenv("HOST_ETC", "/host/etc")

	configPath := filepath.Join(t.TempDir(), "empty_conf.yaml")
	require.NoError(t, os.WriteFile(configPath, nil, 0o600))
	Datadog().(pkgconfigmodel.BuildableConfig).SetConfigFile(configPath)

	err := LoadDatadog(Datadog(), secretsmock.New(t), delegatedauthmock.New(t), nil)
	require.NoError(t, err)

	for _, name := range []string{
		"system_probe_config.apt_config_dir",
		"system_probe_config.yum_repos_dir",
		"system_probe_config.zypper_repos_dir",
	} {
		val := SystemProbe().GetString(name)
		require.True(t, strings.HasPrefix(val, "/host/etc"), "expected %s to be rooted under HOST_ETC, got %q", name, val)
	}
}
