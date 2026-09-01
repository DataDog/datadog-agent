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
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/ebpf-profiler/libpf"
	"go.opentelemetry.io/ebpf-profiler/libpf/pfelf"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
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

// The local-dynamic access model needs the TLS variable to be non-preemptible,
// which is what hiding it does; the resolver then has to find it in .symtab.
const otelTLSFixtureDSOHidden = `
__attribute__((visibility("hidden"))) __thread void *otel_thread_ctx_v1;

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

// TestResolveOTelTLSAccessModels pins the access model each TLS dialect and
// visibility lands on, over the matrix the upstream profiler builds for its own
// integration tests (processcontext/integrationtests/testdata/Makefile in
// open-telemetry/opentelemetry-ebpf-profiler#1229). Which model a plain build
// produces is a property of the toolchain rather than of the source, so without
// pinning them here a compiler upgrade can quietly leave one uncovered.
func TestResolveOTelTLSAccessModels(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dialect, altDialect := "desc", "trad"
	if runtime.GOARCH == "amd64" {
		dialect, altDialect = "gnu2", "gnu"
	}

	tests := []struct {
		name   string
		source string
		flags  []string
		access tlsAccess
	}{
		{
			name:   "general-dynamic-desc",
			source: otelTLSFixtureDSO,
			flags:  []string{"-mtls-dialect=" + dialect},
			access: accessTLSDesc,
		},
		{
			name:   "general-dynamic-gnu",
			source: otelTLSFixtureDSO,
			flags:  []string{"-mtls-dialect=" + altDialect},
			access: accessGlobalDynamic,
		},
		{
			name:   "initial-exec",
			source: otelTLSFixtureDSO,
			flags:  []string{"-ftls-model=initial-exec"},
			access: accessInitialExec,
		},
		{
			name:   "local-dynamic",
			source: otelTLSFixtureDSOHidden,
			flags:  []string{"-ftls-model=local-dynamic", "-mtls-dialect=" + altDialect},
			access: accessLocalDynamic,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			flags := append([]string{"-shared", "-fPIC"}, test.flags...)
			// -mtls-dialect is a GCC option, and not every target the agent
			// builds on takes every dialect.
			lib, ok := compileOptionalOTelTLSFixture(t, dir, "libotel_fixture.so", test.source, flags...)
			if !ok {
				t.Skipf("C toolchain cannot build the %s fixture", test.name)
			}

			ef, err := pfelf.Open(lib)
			require.NoError(t, err)
			defer ef.Close()

			sym := findOTelTLSSymbol(lib)
			require.NotNil(t, sym, "%s exports no %s symbol", lib, otelTLSSymbolName)
			access, err := resolveTLSAccess(ef, &libpf.Symbol{
				Address: libpf.SymbolValue(sym.Value),
				Size:    sym.Size,
			})
			require.NoError(t, err)
			require.Equal(t, test.access, access.access)
		})
	}
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

// TestResolveOTelTLSMuslDTV is the whole of the musl coverage, and the reason
// the functional tests build glibc testers only: every access model resolves
// the same way whatever the libc, and the one thing musl changes is the DTV
// layout its libc reports. Three things here appear nowhere else in the suite:
// an 8-byte DTV entry rather than glibc's 16, a *negative* offset from the
// thread pointer (glibc reports 0 on arm64 and 8 on x86_64, so nothing else
// exercises the sign of otel_dtv_info_t.offset through serialization), and the
// musl branch of the upstream __tls_get_addr decoder.
func TestResolveOTelTLSMuslDTV(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	// No -mtls-dialect or -ftls-model: a dlopen'd module cannot live in the
	// static TLS block, so musl reaches it through the DTV whatever the dialect.
	lib, ok := compileOptionalOTelTLSFixtureWithCompiler(t, "musl-gcc", dir, "libotel_fixture.so", otelTLSFixtureDSO, "-shared", "-fPIC")
	if !ok {
		t.Skip("musl-gcc is not available or cannot build the shared musl fixture")
	}
	bin, ok := compileOptionalOTelTLSFixtureWithCompiler(t, "musl-gcc", dir, "dlopen-musl-main", otelTLSFixtureDlopenMain, "-ldl")
	if !ok {
		t.Skip("musl-gcc cannot build the musl dlopen driver")
	}

	cmd := startOTelTLSFixture(t, bin, lib)
	res, err := resolveOTelTLS(uint32(cmd.Process.Pid), "cpp")
	require.NoError(t, err)

	require.Equal(t, uint32(otelRuntimeNative), res.runtimeLang)
	// Asserted rather than assumed: musl having no static TLS surplus for
	// dlopen is an allocation policy, not a guarantee. Should a later musl
	// gain one, module_id would go to 0 and this test would quietly stop
	// covering the DTV at all, so it has to fail instead.
	require.NotZero(t, res.moduleID, "a dlopen'd musl DSO must resolve through the DTV")
	require.Equal(t, uint32(8), res.dtvInfo.multiplier, "musl DTV entries are 8 bytes, glibc's are 16")

	if runtime.GOARCH == "arm64" {
		require.Negative(t, res.dtvInfo.offset, "musl puts the DTV below the thread pointer on arm64")
		buf := serializeOTelTLSValue(res)
		require.Len(t, buf, otelTLSValueSize)
		require.Equal(t, uint64(res.dtvInfo.offset), binary.NativeEndian.Uint64(buf[16:24]),
			"the negative offset must reach struct otel_dtv_info_t's s64 sign-extended")
	}
}

// TestVisitRelocations uses the DSO fixture rather than the main-executable
// one: a TLS variable used only within the main executable often compiles to
// local-exec with no relocation at all, whereas a shared library's exported TLS
// variable always needs one. A relocation on symbol index 0 ("") counts: that
// is the local-dynamic model.
func TestVisitRelocations(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	lib := compileOTelTLSFixture(t, dir, "reloc-check.so", otelTLSFixtureDSO, "-shared", "-fPIC")
	requireDynamicTLSSymbol(t, lib)

	ef, err := pfelf.Open(lib)
	require.NoError(t, err)
	defer ef.Close()

	var found bool
	err = visitRelocations(ef, func(_ ElfReloc, symName string, _ RelocType) bool {
		if symName == otelTLSSymbolName || symName == "" {
			found = true
			return false
		}
		return true
	}, RelTLSDESC|RelDTPMOD64|RelTPOFF64)
	require.NoError(t, err)
	require.True(t, found, "expected at least one TLS relocation for %s", otelTLSSymbolName)
}

// TestDynValue checks the DF_1_PIE read that tells a PIE from a shared library.
func TestDynValue(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	dir := t.TempDir()
	lib := compileOTelTLSFixture(t, dir, "dynvalue-check.so", otelTLSFixtureDSO, "-shared", "-fPIC")

	ef, err := pfelf.Open(lib)
	require.NoError(t, err)
	defer ef.Close()

	// A shared library carries no DF_1_PIE, so isExecutable must fall through to
	// its PT_INTERP check -- and find none either.
	vals, err := dynValue(ef, elf.DT_FLAGS_1)
	require.NoError(t, err)
	for _, v := range vals {
		require.Zero(t, v&uint64(elf.DF_1_PIE), "%s is flagged DF_1_PIE", lib)
	}
	require.False(t, isExecutable(ef))
}

// TestELFLoadBias checks the bias against an independent ground truth: an
// object's PT_LOAD segment at file offset 0 carries the ELF header, so once
// biased, its vaddr must point at the ELF magic in the live process. A bias off
// by a page, or anchored on the wrong segment, fails here.
func TestELFLoadBias(t *testing.T) {
	skipUnsupportedOTelTLSArch(t)

	self := uint32(os.Getpid())
	target, err := openOTelTargetProcess(self)
	require.NoError(t, err)
	grouped, order, err := target.groupedReadableFileMaps()
	require.NoError(t, err)

	mem, err := procfs.OpenMem(self)
	require.NoError(t, err)
	defer mem.Close()

	checked := 0
	for _, path := range order {
		headerVaddr, ok := elfHeaderVaddr(t, target.fsPath(path))
		if !ok {
			continue
		}

		elfFile, err := pfelf.Open(target.fsPath(path))
		require.NoError(t, err)
		bias, err := elfLoadBias(elfFile, grouped[path])
		elfFile.Close()
		require.NoError(t, err, "load bias of %s", path)

		word, err := mem.ReadUint64(bias + headerVaddr)
		require.NoError(t, err, "%s: nothing mapped at bias %#x + %#x", path, bias, headerVaddr)
		// Little-endian only, which openOTelELF already enforces.
		require.Equal(t, uint32(0x464c457f), uint32(word), "%s: no ELF header at bias %#x", path, bias)
		checked++
	}
	require.NotZero(t, checked, "no shared object of the test process could be checked")
}

// elfHeaderVaddr returns the link-time address of the ELF header of a shared
// object with an executable mapping, i.e. the vaddr of its PT_LOAD segment at
// file offset 0. It reports false for anything elfLoadBias is not asked about.
func elfHeaderVaddr(t *testing.T, path string) (uint64, bool) {
	t.Helper()

	elfFile, err := openOTelELF(path)
	if err != nil {
		return 0, false
	}
	defer elfFile.Close()

	if elfFile.Type != elf.ET_DYN {
		return 0, false
	}
	for _, prog := range elfFile.Progs {
		if prog.Type == elf.PT_LOAD && prog.Off == 0 && prog.Filesz > 4 {
			return prog.Vaddr, true
		}
	}
	return 0, false
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
