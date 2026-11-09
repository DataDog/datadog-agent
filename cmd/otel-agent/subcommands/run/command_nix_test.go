// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && otlp && test

package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
)

// writeCoreConf creates a datadog.yaml in a fresh temp dir and returns its path.
func writeCoreConf(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "datadog.yaml")
	if err := os.WriteFile(path, []byte("api_key: 0000001\n"), 0o600); err != nil {
		t.Fatalf("writing core config: %v", err)
	}
	return path
}

func TestDefaultCoreConfPath(t *testing.T) {
	tests := []struct {
		name string
		// candidate is built from the temp dir when nil.
		candidate  func(t *testing.T) string
		standalone string
		wantFound  bool
	}{
		{
			name:      "existing file is used",
			candidate: writeCoreConf,
			wantFound: true,
		},
		{
			name: "missing file is not used",
			candidate: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "datadog.yaml")
			},
		},
		{
			name: "directory is not used",
			candidate: func(t *testing.T) string {
				return t.TempDir()
			},
		},
		{
			name:       "standalone opts out even when the file exists",
			candidate:  writeCoreConf,
			standalone: "true",
		},
		{
			name:       "standalone=false still uses the file",
			candidate:  writeCoreConf,
			standalone: "false",
			wantFound:  true,
		},
		{
			name:       "unparseable standalone value is ignored",
			candidate:  writeCoreConf,
			standalone: "yes-please",
			wantFound:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.standalone != "" {
				t.Setenv("DD_OTEL_STANDALONE", tc.standalone)
			}
			candidate := tc.candidate(t)

			got := defaultCoreConfPath(candidate)

			if tc.wantFound {
				assert.Equal(t, candidate, got)
			} else {
				assert.Empty(t, got)
			}
		})
	}
}

// A symlinked datadog.yaml is what the container entrypoint scripts create
// (cont-init.d/50-ecs.sh and friends symlink datadog-ecs.yaml into place).
func TestDefaultCoreConfPathFollowsSymlink(t *testing.T) {
	target := writeCoreConf(t)
	link := filepath.Join(filepath.Dir(target), "datadog-link.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("creating symlink: %v", err)
	}

	assert.Equal(t, link, defaultCoreConfPath(link))
}

func TestTryToGetDefaultParamsIfMissingKeepsExplicitPath(t *testing.T) {
	// An explicit path wins even when it does not exist, so that a typo surfaces
	// as a config load error rather than being silently swapped for the default.
	p := &cliParams{GlobalParams: &subcommands.GlobalParams{CoreConfPath: "/custom/datadog.yaml"}}

	TryToGetDefaultParamsIfMissing(p)

	assert.Equal(t, "/custom/datadog.yaml", p.CoreConfPath)
}

func TestTryToGetDefaultParamsIfMissingSkipsStandalone(t *testing.T) {
	t.Setenv("DD_OTEL_STANDALONE", "true")
	p := &cliParams{GlobalParams: &subcommands.GlobalParams{}}

	TryToGetDefaultParamsIfMissing(p)

	assert.Empty(t, p.CoreConfPath)
}
