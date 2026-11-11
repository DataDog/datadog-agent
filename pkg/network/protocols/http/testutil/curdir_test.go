// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveCurDir(t *testing.T) {
	const (
		staging = "/opt/kmt-ramfs/system-probe-tests"
		pkgDir  = staging + "/pkg/discovery/module"
	)
	tests := []struct {
		name       string
		caller     string
		executable string
		cwd        string
		srcDir     string
		workspace  string
		want       string
	}{
		{
			name:       "trimpath basename uses the staged test binary",
			caller:     "impl_linux_test.go",
			executable: pkgDir + "/testsuite",
			cwd:        staging,
			want:       pkgDir,
		},
		{
			name:       "full caller path is relocated onto the working tree",
			caller:     "/repo/pkg/discovery/module/impl_linux_test.go",
			executable: "/tmp/go-build/test.test",
			cwd:        staging,
			want:       pkgDir,
		},
		{
			name:      "bazel runfiles",
			caller:    "/repo/pkg/discovery/module/impl_linux_test.go",
			cwd:       staging,
			srcDir:    "/tmp/runfiles",
			workspace: "datadog-agent",
			want:      "/tmp/runfiles/datadog-agent/pkg/discovery/module",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCurDir(tt.caller, tt.executable, tt.cwd, tt.srcDir, tt.workspace)
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
