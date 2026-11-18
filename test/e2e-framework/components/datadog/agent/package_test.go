// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tifos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
)

func TestGetPackagePathArch(t *testing.T) {
	// Package names as the packaging jobs produce them: deb names the architectures
	// amd64/arm64, rpm x86_64/aarch64.
	localPath := t.TempDir()
	pkgDir := filepath.Join(localPath, "pkg")
	require.NoError(t, os.Mkdir(pkgDir, 0o755))
	for _, name := range []string{
		"datadog-agent_7.80.0-1_amd64.deb",
		"datadog-agent_7.80.0-1_arm64.deb",
		"datadog-agent-7.80.0-1.x86_64.rpm",
		"datadog-agent-7.80.0-1.aarch64.rpm",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(pkgDir, name), nil, 0o644))
	}

	tests := []struct {
		flavor tifos.Flavor
		arch   tifos.Architecture
		want   string
	}{
		{tifos.Ubuntu, tifos.AMD64Arch, "datadog-agent_7.80.0-1_amd64.deb"},
		{tifos.Ubuntu, tifos.ARM64Arch, "datadog-agent_7.80.0-1_arm64.deb"},
		{tifos.CentOS, tifos.AMD64Arch, "datadog-agent-7.80.0-1.x86_64.rpm"},
		{tifos.CentOS, tifos.ARM64Arch, "datadog-agent-7.80.0-1.aarch64.rpm"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got, err := GetPackagePath(localPath, tt.flavor, "", tt.arch, "")
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(pkgDir, tt.want), got)
		})
	}
}
