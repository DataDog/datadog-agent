// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && !serverless

package setup

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// TestFixupInitSystemProbeRunsAfterConfigIsReady is a regression test for a bug where
// fixupInitSystemProbe read/wrote system-probe config keys before BuildSchema() marked the
// config ready for use. Under HOST_ETC (any containerized deployment), the guarded getters
// silently no-op instead of applying the HOST_ETC path rewrite, so apt/yum/zypper repo dirs
// were never adjusted to point under the host filesystem.
func TestFixupInitSystemProbeRunsAfterConfigIsReady(t *testing.T) {
	origDatadog := Datadog().(pkgconfigmodel.BuildableConfig)
	origSystemProbe := SystemProbe().(pkgconfigmodel.BuildableConfig)
	t.Cleanup(func() {
		SetDatadog(origDatadog)         // nolint: forbidigo // restoring the singleton after the test
		SetSystemProbe(origSystemProbe) // nolint: forbidigo // restoring the singleton after the test
	})

	t.Setenv("HOST_ETC", "/host/etc")

	InitConfigObjects()

	for _, name := range []string{
		"system_probe_config.apt_config_dir",
		"system_probe_config.yum_repos_dir",
		"system_probe_config.zypper_repos_dir",
	} {
		val := SystemProbe().GetString(name)
		require.True(t, strings.HasPrefix(val, "/host/etc"), "expected %s to be rooted under HOST_ETC, got %q", name, val)
	}
}
