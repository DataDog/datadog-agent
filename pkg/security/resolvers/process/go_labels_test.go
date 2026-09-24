// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process holds process related files
package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// buildGoLabelsTestBinary builds the test program with the given environment and
// linker flags, and returns the path to the resulting binary.
func buildGoLabelsTestBinary(t *testing.T, dir, name string, env []string, args ...string) string {
	t.Helper()

	out := filepath.Join(dir, name)
	cmd := exec.Command("go", append(append([]string{"build", "-o", out}, args...), "./main.go")...)
	cmd.Dir = "testdata"
	cmd.Env = append(os.Environ(), env...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %v %v failed: %s\n%s", env, args, err, output)
	}
	return out
}

type goLabelsFixtureKind string

const (
	goLabelsFixtureNoCGO  goLabelsFixtureKind = "NOCGO"
	goLabelsFixtureCGO    goLabelsFixtureKind = "CGO"
	goLabelsFixtureCGOPIE goLabelsFixtureKind = "CGO_PIE"
)

func isBazelTest() bool {
	return os.Getenv("TEST_SRCDIR") != "" || os.Getenv("RUNFILES_DIR") != "" || os.Getenv("RUNFILES_MANIFEST_FILE") != ""
}

func bazelGoLabelsTestBinary(t *testing.T, fixture goLabelsFixtureKind, stripped bool) string {
	t.Helper()

	suffix := "PLAIN"
	if stripped {
		suffix = "STRIPPED"
	}
	envName := "GO_LABELS_TEST_FIXTURE_" + string(fixture) + "_" + suffix
	loc := os.Getenv(envName)
	require.NotEmptyf(t, loc, "expected %s to be set by the Bazel test rule", envName)

	path, err := runfiles.Rlocation(loc)
	require.NoError(t, err)
	return path
}

func goLabelsTestBinary(t *testing.T, dir, name string, fixture goLabelsFixtureKind, stripped bool, env []string, args ...string) string {
	t.Helper()

	if isBazelTest() {
		return bazelGoLabelsTestBinary(t, fixture, stripped)
	}
	return buildGoLabelsTestBinary(t, dir, name, env, args...)
}

// hasSymtab reports whether the binary still carries a symbol table.
func hasSymtab(t *testing.T, path string) bool {
	t.Helper()

	f, err := safeelf.Open(path)
	require.NoError(t, err)
	defer f.Close()

	_, err = f.Symbols()
	return err == nil
}

// TestStripGoVersion checks that the " X:<experiments>" suffix the toolchain
// appends to the buildinfo version string.
func TestStripGoVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "go1.25.11", want: "go1.25.11"},
		{name: "single experiment", in: "go1.25.11 X:loopvar", want: "go1.25.11"},
		{name: "multiple experiments", in: "go1.21.0 X:boringcrypto,arenas", want: "go1.21.0"},
		{name: "no patch", in: "go1.13 X:loopvar", want: "go1.13"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stripGoVersion(tt.in))
		})
	}
}

// TestExtractTLSGOffset checks that the g TLS offset is recovered identically
// from stripped and unstripped binaries. Stripping removes .symtab but not
// .gopclntab, so an implementation that looks up runtime.tlsg in the symbol
// table silently falls back to a hardcoded offset on stripped binaries — which
// is what most production Go images are.
func TestExtractTLSGOffset(t *testing.T) {
	if !isBazelTest() {
		if _, err := exec.LookPath("go"); err != nil {
			t.Skip("go toolchain not available")
		}
	}

	variants := []struct {
		name    string
		fixture goLabelsFixtureKind
		env     []string
		args    []string
	}{
		{name: "nocgo", fixture: goLabelsFixtureNoCGO, env: []string{"CGO_ENABLED=0"}},
		{name: "cgo", fixture: goLabelsFixtureCGO, env: []string{"CGO_ENABLED=1"}},
		{name: "cgo_pie", fixture: goLabelsFixtureCGOPIE, env: []string{"CGO_ENABLED=1"}, args: []string{"-buildmode=pie"}},
	}

	offsetOf := func(t *testing.T, path string) int32 {
		t.Helper()

		f, err := pfelf.Open(path)
		require.NoError(t, err)
		defer f.Close()

		offset, err := extractTLSGOffset(f)
		require.NoError(t, err)
		return offset
	}

	dir := t.TempDir()
	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			plain := goLabelsTestBinary(t, dir, v.name, v.fixture, false, v.env, v.args...)
			stripped := goLabelsTestBinary(t, dir, v.name+"_stripped", v.fixture, true, v.env,
				append(append([]string{}, v.args...), "-ldflags=-s -w")...)

			require.True(t, hasSymtab(t, plain), "unstripped binary should have a symbol table")
			require.False(t, hasSymtab(t, stripped), "stripped binary should not have a symbol table")

			want := offsetOf(t, plain)
			assert.Equal(t, want, offsetOf(t, stripped),
				"stripping must not change the recovered TLS offset")

			switch runtime.GOARCH {
			case "amd64":
				// g always lives in TLS, below the thread pointer.
				assert.Negative(t, want)
				assert.Greater(t, want, int32(-4096))
			case "arm64":
				if v.name == "nocgo" {
					// runtime.save_g is a no-op when runtime.iscgo is false, so
					// the TLS slot is never written: 0 tells eBPF to read R28.
					assert.Zero(t, want)
				} else {
					assert.Positive(t, want)
					assert.Less(t, want, int32(4096))
				}
			}
		})
	}
}
