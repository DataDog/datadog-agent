// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/tls"
)

// TestLiveDlopenBigTLSDescResolution dlopens a TLSDESC module too large for the
// static-TLS surplus and resolves it the way resolveTLSOffsets does. Its
// tls_index often lives on a heap below 4GiB, which a magnitude cut on the
// TLSDESC argument would mistake for a static offset.
//
// Needs the otel_tls tester artifacts: set OTEL_TLS_ARTIFACT_DIR or build them
// under bazel-bin. Skips otherwise.
func TestLiveDlopenBigTLSDescResolution(t *testing.T) {
	driver, so, ok := findOTelTLSBigArtifacts()
	if !ok {
		t.Skip("otel_tls_dlopen_glibc / libotel_tls_glibc_big.so not built")
	}

	continueFile := filepath.Join(t.TempDir(), "continue")
	cmd := exec.Command(driver, so, "otel-span-open", "deadbeef", "204", filepath.Join(t.TempDir(), "touch"))
	cmd.Env = append(os.Environ(), "SPAN_TESTER_CONTINUE_FILE="+continueFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	defer func() {
		_ = os.WriteFile(continueFile, nil, 0o600)
		_ = cmd.Wait()
	}()

	pid := uint32(cmd.Process.Pid)
	require.Eventually(t, func() bool {
		return mappedObjectContains(pid, filepath.Base(so))
	}, 5*time.Second, 20*time.Millisecond, "fixture never appeared in maps")

	var target otelTargetProcess
	target.reset(pid)
	module, sym, err := target.findOTelTLSModule(otelTLSSymbolName)
	require.NoError(t, err)
	defer module.file.Close()

	tlsVar, err := tls.Resolve(module.file, sym)
	require.NoError(t, err)

	res, err := target.locateOTelTLS(module.loadBias, tlsVar)
	require.NoError(t, err)
	t.Logf("var=%v module=%d tlsOffset=%#x dtv=%+v", tlsVar, res.moduleID, res.tlsOffset, res.dtvInfo)

	require.NotZero(t, res.moduleID, "dlopen of _big must resolve through the DTV")
	require.Zero(t, res.tlsOffset, "otel_thread_ctx_v1 is at offset 0 of the TLS block")
	require.NotZero(t, res.dtvInfo.multiplier)
}

func findOTelTLSBigArtifacts() (driver, so string, ok bool) {
	candidates := []string{}
	if dir := os.Getenv("OTEL_TLS_ARTIFACT_DIR"); dir != "" {
		candidates = append(candidates, dir)
	}
	candidates = append(candidates,
		"bazel-bin/pkg/security/tests/syscall_tester/c",
		filepath.Join(os.Getenv("TEST_SRCDIR"), "pkg/security/tests/syscall_tester/c"),
	)
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for range 8 {
			candidates = append(candidates, filepath.Join(dir, "bazel-bin/pkg/security/tests/syscall_tester/c"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	for _, dir := range candidates {
		d := filepath.Join(dir, "otel_tls_dlopen_glibc")
		s := filepath.Join(dir, "libotel_tls_glibc_big.so")
		if st, err := os.Stat(d); err == nil && !st.IsDir() {
			if st, err := os.Stat(s); err == nil && !st.IsDir() {
				return d, s, true
			}
		}
	}
	return "", "", false
}

func mappedObjectContains(pid uint32, base string) bool {
	f, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.Contains(sc.Text(), base) {
			return true
		}
	}
	return false
}
