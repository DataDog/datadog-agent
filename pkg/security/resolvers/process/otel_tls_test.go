// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"bytes"
	"debug/elf"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
	"github.com/stretchr/testify/require"
)

const otelTLSFixtureMain = `
#include <stdio.h>
#include <unistd.h>

__attribute__((visibility("default"))) __thread void *otel_thread_ctx_v1;

__attribute__((visibility("default"))) void touch_otel_thread_ctx_v1(void) {
    otel_thread_ctx_v1 = &otel_thread_ctx_v1;
}

int main(void) {
    touch_otel_thread_ctx_v1();
    puts("ready");
    fflush(stdout);
    sleep(60);
    return 0;
}
`

const otelTLSFixtureDSO = `
__attribute__((visibility("default"))) __thread void *otel_thread_ctx_v1;

__attribute__((visibility("default"))) void touch_otel_thread_ctx_v1(void) {
    otel_thread_ctx_v1 = &otel_thread_ctx_v1;
}
`

const otelTLSFixtureDlopenMain = `
#define _GNU_SOURCE
#include <dlfcn.h>
#include <stdio.h>
#include <unistd.h>

typedef void (*touch_fn)(void);

int main(int argc, char **argv) {
    if (argc != 2) {
        return 2;
    }

    void *handle = dlopen(argv[1], RTLD_NOW);
    if (handle == NULL) {
        fprintf(stderr, "dlopen failed: %s\n", dlerror());
        return 3;
    }

    touch_fn touch = (touch_fn)dlsym(handle, "touch_otel_thread_ctx_v1");
    if (touch == NULL) {
        fprintf(stderr, "dlsym failed: %s\n", dlerror());
        return 4;
    }

    touch();
    puts("ready");
    fflush(stdout);
    sleep(60);
    return 0;
}
`

func TestResolveOTelTLSDynamicMain(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	bin := compileOTelTLSFixture(t, dir, "dynamic-main", otelTLSFixtureMain, "-rdynamic")
	requireDynamicTLSSymbol(t, bin)

	cmd := startOTelTLSFixture(t, bin)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	require.Equal(t, otelTLSModeLinkMap, res.mode)
	require.NotZero(t, res.dtDebugValueAddr)
	require.Equal(t, uint64(8), res.targetSymbolSize)
	require.LessOrEqual(t, res.targetSymbolOffset+res.targetSymbolSize, res.targetTLSMemsz)
	requireValidDynamicLookup(t, res)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func TestResolveOTelTLSDlopenDSO(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	lib := compileOTelTLSFixture(t, dir, "libotel_fixture.so", otelTLSFixtureDSO, "-shared", "-fPIC")
	requireDynamicTLSSymbol(t, lib)
	bin := compileOTelTLSFixture(t, dir, "dlopen-main", otelTLSFixtureDlopenMain, "-ldl")

	cmd := startOTelTLSFixture(t, bin, lib)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	require.Equal(t, otelTLSModeLinkMap, res.mode)
	require.NotZero(t, res.dtDebugValueAddr)
	require.NotZero(t, res.targetLoadBias)
	require.Equal(t, uint64(8), res.targetSymbolSize)
	require.LessOrEqual(t, res.targetSymbolOffset+res.targetSymbolSize, res.targetTLSMemsz)
	requireValidDynamicLookup(t, res)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func TestResolveOTelTLSStaticPIEMain(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	bin, ok := compileOptionalOTelTLSFixture(t, dir, "static-pie-main", otelTLSFixtureMain, "-static-pie", "-Wl,--export-dynamic")
	if !ok {
		t.Skip("C toolchain cannot build static PIE fixture")
	}
	requireDynamicTLSSymbol(t, bin)
	requireNoInterpreter(t, bin)

	cmd := startOTelTLSFixture(t, bin)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	require.Equal(t, otelTLSModeStaticMain, res.mode)
	require.Zero(t, res.dtDebugValueAddr)
	require.Equal(t, uint64(8), res.targetSymbolSize)
	require.LessOrEqual(t, res.targetSymbolOffset+res.targetSymbolSize, res.targetTLSMemsz)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func requireValidDynamicLookup(t *testing.T, res otelTLSResolution) {
	t.Helper()

	if res.reconstructModuleIDs == 0 {
		require.NotZero(t, res.linkMapLTLSModIDOffset)
		require.NotZero(t, res.linkMapLTLSTPOffsetOffset)
		require.NotZero(t, res.dtvEntrySize)
		return
	}

	require.NotZero(t, res.tlsModuleCount)
	require.True(t, res.tlsModuleHashBits[otelTLSHashSlot(res.targetLoadBias, res.tlsModuleHashSeed)>>6] != 0)
}

func skipUnsupportedOTelTLSArch(t *testing.T) {
	t.Helper()
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skipf("OTel TLS resolver supports amd64/arm64, got %s", runtime.GOARCH)
	}
}

func compileOTelTLSFixture(t *testing.T, dir string, name string, source string, args ...string) string {
	t.Helper()

	out, ok := compileOptionalOTelTLSFixture(t, dir, name, source, args...)
	require.True(t, ok)
	return out
}

func compileOptionalOTelTLSFixture(t *testing.T, dir string, name string, source string, args ...string) (string, bool) {
	t.Helper()

	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc is not available")
	}

	src := filepath.Join(dir, name+".c")
	out := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(src, []byte(source), 0o644))

	cmdArgs := []string{"-O0", "-g", "-o", out, src}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(cc, cmdArgs...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Logf("%s failed: %s\n%s", strings.Join(append([]string{cc}, cmdArgs...), " "), err, output.String())
		return "", false
	}

	return out, true
}

func startOTelTLSFixture(t *testing.T, bin string, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(bin, args...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	ready := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			ready <- scanner.Text()
			return
		}
		ready <- ""
	}()

	select {
	case line := <-ready:
		require.Equal(t, "ready", line, "fixture stderr: %s pid=%s", stderr.String(), strconv.Itoa(cmd.Process.Pid))
	case <-time.After(5 * time.Second):
		t.Fatalf("fixture did not become ready; stderr: %s", stderr.String())
	}

	return cmd
}

func requireDynamicTLSSymbol(t *testing.T, path string) {
	t.Helper()
	require.True(t, hasDynamicTLSSymbol(path), "%s does not export %s as STT_TLS in .dynsym", path, otelTLSSymbolName)
}

func requireNoInterpreter(t *testing.T, path string) {
	t.Helper()

	file, err := safeelf.Open(path)
	require.NoError(t, err)
	defer file.Close()

	for _, prog := range file.Progs {
		require.NotEqual(t, elf.PT_INTERP, prog.Type, "%s unexpectedly has a PT_INTERP segment", path)
	}
}

func hasDynamicTLSSymbol(path string) bool {
	file, err := safeelf.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	syms, err := file.DynamicSymbols()
	if err != nil {
		return false
	}

	for _, sym := range syms {
		if sym.Name == otelTLSSymbolName && sym.Section != elf.SHN_UNDEF && safeelf.ST_TYPE(sym.Info) == elf.STT_TLS {
			return true
		}
	}
	return false
}
