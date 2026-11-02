// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"bufio"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
)

// TestLiveDlopenBigTLSDescClassification reproduces the KMT failure mode of
// PR #59318 without eBPF: dlopen a TLSDESC module too large for the static-TLS
// surplus, then classify its GOT slot the same way attachOTelTLS does. The
// old `> 0xffffffff` cut called the heap-resident tls_index a static offset;
// the sign-based (amd64) / resolver-based (arm64) discriminant must not.
//
// Requires the bazel-built tester artifacts next to this package's cwd, or
// under the well-known bazel-bin path. Skips otherwise.
func TestLiveDlopenBigTLSDescClassification(t *testing.T) {
	driver, so, ok := findOTelTLSBigArtifacts()
	if !ok {
		t.Skip("otel_tls_dlopen_glibc / libotel_tls_glibc_big.so not built")
	}

	tlsdescOff, err := findTLSDESCOffset(so, otelTLSSymbolName)
	require.NoError(t, err)

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

	bias, err := loadBiasOf(pid, filepath.Base(so))
	require.NoError(t, err)

	mem, err := procfs.OpenMem(pid)
	require.NoError(t, err)
	defer mem.Close()

	resolver, err := mem.ReadUint64(bias + tlsdescOff)
	require.NoError(t, err)
	arg, err := mem.ReadUint64(bias + tlsdescOff + 8)
	require.NoError(t, err)
	require.NotZero(t, resolver, "TLSDESC not yet relocated")

	static, moduleID, tlsOffset, err := classifyOTelTLSDesc(mem, resolver, arg)
	require.NoError(t, err)
	t.Logf("bias=%#x resolver=%#x arg=%#x static=%v module=%d offset=%#x",
		bias, resolver, arg, static, moduleID, tlsOffset)

	require.False(t, static, "dlopen of _big must resolve through the DTV")
	require.NotZero(t, moduleID)
	require.Equal(t, uint64(0), tlsOffset, "otel_thread_ctx_v1 is at offset 0 of the TLS block")
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
		// Climb toward the repo root from this package.
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

func findTLSDESCOffset(soPath, symbol string) (uint64, error) {
	f, err := elf.Open(soPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// debug/elf's Symbols/DynamicSymbols omit the null symbol at index 0, so a
	// reloc's symbol index N refers to syms[N-1].
	syms, err := f.DynamicSymbols()
	if err != nil {
		return 0, err
	}
	for _, section := range f.Sections {
		if section.Type != elf.SHT_RELA {
			continue
		}
		data, err := section.Data()
		if err != nil {
			continue
		}
		for off := 0; off+24 <= len(data); off += 24 {
			rOff := binary.LittleEndian.Uint64(data[off : off+8])
			info := binary.LittleEndian.Uint64(data[off+8 : off+16])
			typ := elf.R_TYPE64(info)
			if typ != uint32(elf.R_X86_64_TLSDESC) && typ != uint32(elf.R_AARCH64_TLSDESC) {
				continue
			}
			symIdx := elf.R_SYM64(info)
			if symIdx == 0 || int(symIdx) > len(syms) {
				continue
			}
			if syms[symIdx-1].Name == symbol {
				return rOff, nil
			}
		}
	}
	return 0, fmt.Errorf("no TLSDESC reloc for %s", symbol)
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

func loadBiasOf(pid uint32, base string) (uint64, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.Contains(line, base) || !strings.Contains(line, "r-xp") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		startEnd := strings.SplitN(fields[0], "-", 2)
		if len(startEnd) != 2 {
			continue
		}
		start, err := strconv.ParseUint(startEnd[0], 16, 64)
		if err != nil {
			return 0, err
		}
		fileOff, err := strconv.ParseUint(fields[2], 16, 64)
		if err != nil {
			return 0, err
		}
		// Executable PT_LOAD of our fixture is linked at vaddr 0x1000 with
		// file offset 0x1000, so bias = start - 0x1000 when fileOff is 0x1000.
		return start - fileOff, nil
	}
	return 0, fmt.Errorf("no executable mapping for %s", base)
}
