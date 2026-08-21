// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"bytes"
	"debug/elf" //nolint:depguard
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

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

	// The access model picked for a main executable's own TLS variable depends
	// on the compiler/glibc version, so only assert resolution completed.
	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
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
	// glibc >= 2.34 can relax a dlopen'd module's TLSDESC access to a
	// static-like TP-relative offset ("surplus TLS", moduleID == 0), so whether
	// this resolves via the DTV depends on the glibc running the test.
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func TestResolveOTelTLSStaticPIEMain(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	bin, ok := compileOptionalOTelTLSFixture(t, dir, "static-pie-main", otelTLSFixtureMain, "-static-pie", "-Wl,--export-dynamic-symbol=otel_thread_ctx_v1")
	if !ok {
		t.Skip("C toolchain cannot build static PIE fixture")
	}
	requireDynamicTLSSymbol(t, bin)
	requireNoInterpreter(t, bin)

	cmd := startOTelTLSFixture(t, bin)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	// No dynamic linker at all: must resolve to local-exec (module_id == 0).
	require.Zero(t, res.moduleID)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func TestResolveOTelTLSStaticNonPIEMainSymtab(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	bin, ok := compileOptionalOTelTLSFixture(t, dir, "static-nopie-main", otelTLSFixtureMain, "-static", "-no-pie")
	if !ok {
		t.Skip("C toolchain cannot build static non-PIE fixture")
	}
	requireNoDynamicTLSSymbol(t, bin)
	requireStaticTLSSymbol(t, bin)
	requireNoInterpreter(t, bin)

	cmd := startOTelTLSFixture(t, bin)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	require.Zero(t, res.moduleID)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

func TestResolveOTelTLSStaticMuslNonPIEMainSymtab(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	bin, ok := compileOptionalOTelTLSFixtureWithCompiler(t, "musl-gcc", dir, "static-musl-nopie-main", otelTLSFixtureMain, "-static", "-no-pie")
	if !ok {
		t.Skip("musl-gcc is not available or cannot build static musl fixture")
	}
	requireNoDynamicTLSSymbol(t, bin)
	requireStaticTLSSymbol(t, bin)
	requireNoInterpreter(t, bin)

	cmd := startOTelTLSFixture(t, bin)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	// Fully static: local-exec, as in the glibc case above. musl only changes
	// the DTV layout, which module_id == 0 never consults.
	require.Zero(t, res.moduleID)
	require.Len(t, serializeOTelTLSValue(res), otelTLSValueSize)
}

// TestSafeELFVisitRelocations uses the DSO fixture rather than the
// main-executable one: a TLS variable used only within the main executable
// often compiles to local-exec with no relocation at all, whereas a shared
// library's exported TLS variable always needs one. A relocation on symbol
// index 0 ("") counts: that is the local-dynamic model.
func TestSafeELFVisitRelocations(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	lib := compileOTelTLSFixture(t, dir, "reloc-check.so", otelTLSFixtureDSO, "-shared", "-fPIC")
	requireDynamicTLSSymbol(t, lib)

	ef, err := safeelf.Open(lib)
	require.NoError(t, err)
	defer ef.Close()

	var found bool
	err = ef.VisitRelocations(func(_ safeelf.ElfReloc, symName string, _ safeelf.RelocType) bool {
		if symName == otelTLSSymbolName || symName == "" {
			found = true
			return false
		}
		return true
	}, safeelf.RelTLSDESC|safeelf.RelDTPMOD64|safeelf.RelTPOFF64)
	require.NoError(t, err)
	require.True(t, found, "expected at least one TLS relocation for %s", otelTLSSymbolName)
}

func TestOTelRemoteMemoryUint64(t *testing.T) {
	var probe uint64 = 0x1122334455667788

	rm := otelRemoteMemory{pidStr: strconv.Itoa(os.Getpid())}
	got, err := rm.Uint64(uint64(uintptr(unsafe.Pointer(&probe))))
	require.NoError(t, err)
	require.Equal(t, probe, got)
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

	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc is not available")
	}
	return compileOptionalOTelTLSFixtureWithCompiler(t, "cc", dir, name, source, args...)
}

func compileOptionalOTelTLSFixtureWithCompiler(t *testing.T, compiler string, dir string, name string, source string, args ...string) (string, bool) {
	t.Helper()

	cc, err := exec.LookPath(compiler)
	if err != nil {
		t.Logf("%s is not available", compiler)
		return "", false
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

// syncBuffer is a bytes.Buffer safe for concurrent Write and String.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func startOTelTLSFixture(t *testing.T, bin string, args ...string) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(bin, args...)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	// The fixture keeps running after "ready", so cmd's stderr-copying goroutine
	// is still writing while we read stderr below.
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
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

func requireNoDynamicTLSSymbol(t *testing.T, path string) {
	t.Helper()
	require.False(t, hasDynamicTLSSymbol(path), "%s unexpectedly exports %s as STT_TLS in .dynsym", path, otelTLSSymbolName)
}

func requireStaticTLSSymbol(t *testing.T, path string) {
	t.Helper()
	require.True(t, hasStaticTLSSymbol(path), "%s does not contain %s as STT_TLS in .symtab", path, otelTLSSymbolName)
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

func hasStaticTLSSymbol(path string) bool {
	file, err := safeelf.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()

	syms, err := file.Symbols()
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
