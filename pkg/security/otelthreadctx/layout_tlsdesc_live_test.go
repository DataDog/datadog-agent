// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelthreadctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/runfiles"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/ebpf-profiler/tls"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// TestLiveDlopenBigTLSDescResolution dlopens a TLSDESC module too large for the
// static-TLS surplus and resolves it the way ResolveLayout does. Its
// tls_index often lives on a heap below 4GiB, which a magnitude cut on the
// TLSDESC argument would mistake for a static offset.
func TestLiveDlopenBigTLSDescResolution(t *testing.T) {
	if !isBazelTest() {
		t.Skip("needs the otel_tls tester artifacts from the Bazel test rule")
	}
	driver := otelTLSTestArtifact(t, "OTEL_TLS_TEST_DLOPEN_DRIVER")
	so := otelTLSTestArtifact(t, "OTEL_TLS_TEST_BIG_SO")

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

	// The library is mapped before dlopen applies its relocations; the process
	// context is only published from inside it, once dlopen has returned.
	pid := uint32(cmd.Process.Pid)
	target := NewTarget(kernel.ProcFSRoot())
	require.Eventually(t, func() bool {
		target.Reset(pid)
		procCtx, err := target.ProcessContext()
		return err == nil && procCtx != nil
	}, 5*time.Second, 20*time.Millisecond, "fixture never published its process context")

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

func isBazelTest() bool {
	return os.Getenv("TEST_SRCDIR") != "" || os.Getenv("RUNFILES_DIR") != "" || os.Getenv("RUNFILES_MANIFEST_FILE") != ""
}

func otelTLSTestArtifact(t *testing.T, envName string) string {
	t.Helper()
	loc := os.Getenv(envName)
	require.NotEmptyf(t, loc, "expected %s to be set by the Bazel test rule", envName)
	path, err := runfiles.Rlocation(loc)
	require.NoError(t, err)
	return path
}
