// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	extensionsPkg "github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/extensions"
)

func TestInstallDDOTExtensionIfEnabled_Disabled(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "false")
	ctx := HookContext{Context: context.Background()}
	err := installDDOTExtensionIfEnabled(ctx, "7.50.0-1", false)
	require.NoError(t, err)
}

func TestInstallDDOTExtensionIfEnabled_Enabled(t *testing.T) {
	t.Setenv("DD_OTELCOLLECTOR_ENABLED", "true")

	tmpDir := t.TempDir()
	extensionsPkg.ExtensionsDBDir = tmpDir

	ctx := HookContext{Context: context.Background(), PackagePath: tmpDir}
	err := extensionsPkg.SetPackage(ctx, agentPackage, "7.50.0-1", false)
	require.NoError(t, err)

	err = installDDOTExtensionIfEnabled(ctx, "7.50.0-1", false)
	// Expect a download error (no real OCI registry), not an env-guard skip
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "otelcollector")
}
