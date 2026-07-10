// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package configstreambootstrap

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestSeedGlobalBuilderResolvesIPCArtifactsNextToDatadogYaml(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "datadog.yaml")
	pkgconfigsetup.InitConfigObjects()
	SeedGlobalBuilder(Settings{CmdHost: "localhost", CmdPort: 5001}, yamlPath)
	require.Equal(t, filepath.Join(dir, "auth_token"), AuthTokenFilepath())
}
