// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package version

import (
	"runtime/debug"
	"testing"
)

func TestVersionNotEmpty(t *testing.T) {
	// In normal test runs (go test), buildVersion returns "dev" because
	// the module is the main module built as (devel). That's fine — we
	// just verify it's never empty.
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
}

func TestModuleVersion(t *testing.T) {
	tests := []struct {
		name string
		info debug.BuildInfo
		want string
	}{
		{
			name: "installed command",
			info: debug.BuildInfo{Main: debug.Module{Path: modulePath, Version: "v0.85.0"}},
			want: "v0.85.0",
		},
		{
			name: "external consumer",
			info: debug.BuildInfo{Deps: []*debug.Module{{Path: modulePath, Version: "v0.85.0"}}},
			want: "v0.85.0",
		},
		{
			name: "local replacement does not report stale release",
			info: debug.BuildInfo{Deps: []*debug.Module{{
				Path: modulePath, Version: "v0.85.0", Replace: &debug.Module{Path: "../rshell"},
			}}},
			want: "dev",
		},
		{
			name: "old module is not this implementation",
			info: debug.BuildInfo{Deps: []*debug.Module{{Path: "github.com/DataDog/rshell", Version: "v0.0.30"}}},
			want: "dev",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := moduleVersion(&tt.info); got != tt.want {
				t.Fatalf("moduleVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}
