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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const goLabelsTestProgram = `package main

import (
	"fmt"
	_ "net"
	_ "os/user"
)

func main() { fmt.Println("hi") }
`

// buildGoLabelsTestBinary builds the test program with the given environment and
// linker flags, and returns the path to the resulting binary.
func buildGoLabelsTestBinary(t *testing.T, dir, name string, env []string, args ...string) string {
	t.Helper()

	out := filepath.Join(dir, name)
	cmd := exec.Command("go", append(append([]string{"build", "-o", out}, args...), ".")...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %v %v failed: %s\n%s", env, args, err, output)
	}
	return out
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

// TestExtractTLSGOffset checks that the g TLS offset is recovered identically
// from stripped and unstripped binaries. Stripping removes .symtab but not
// .gopclntab, so an implementation that looks up runtime.tlsg in the symbol
// table silently falls back to a hardcoded offset on stripped binaries — which
// is what most production Go images are.
func TestExtractTLSGOffset(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	variants := []struct {
		name string
		env  []string
		args []string
	}{
		{name: "nocgo", env: []string{"CGO_ENABLED=0"}},
		{name: "cgo", env: []string{"CGO_ENABLED=1"}},
		{name: "cgo_pie", env: []string{"CGO_ENABLED=1"}, args: []string{"-buildmode=pie"}},
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte(goLabelsTestProgram), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module golabelstest\n\ngo 1.21\n"), 0o600))

	offsetOf := func(t *testing.T, path string) int32 {
		t.Helper()

		f, err := pfelf.Open(path)
		require.NoError(t, err)
		defer f.Close()

		offset, err := extractTLSGOffset(f)
		require.NoError(t, err)
		return offset
	}

	for _, v := range variants {
		t.Run(v.name, func(t *testing.T) {
			plain := buildGoLabelsTestBinary(t, dir, v.name, v.env, v.args...)
			stripped := buildGoLabelsTestBinary(t, dir, v.name+"_stripped", v.env,
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
