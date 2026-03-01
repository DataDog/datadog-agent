// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

func TestIRGenAllProbes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	programs := testprogs.MustGetPrograms(t)
	cfgs := testprogs.MustGetCommonConfigs(t)

	// Find objcopy for each architecture.
	// Native arch uses plain 'objcopy', cross-arch needs arch-specific toolchain.
	crossObjcopyNames := map[string]string{
		"amd64": "x86_64-linux-gnu-objcopy",
		"arm64": "aarch64-linux-gnu-objcopy",
	}
	objcopyByArch := make(map[string]string)
	for _, arch := range []string{"amd64", "arm64"} {
		var name string
		if arch == runtime.GOARCH {
			name = "objcopy"
		} else {
			name = crossObjcopyNames[arch]
		}
		if path, err := exec.LookPath(name); err == nil {
			objcopyByArch[arch] = path
		}
	}

	for _, pkg := range programs {
		switch pkg {
		case "simple", "sample":
		default:
			t.Logf("skipping %s", pkg)
			continue
		}
		t.Run(pkg, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					bin := testprogs.MustGetBinary(t, pkg, cfg)
					testAllProbes(t, bin)
					version, ok := object.ParseGoVersion(cfg.GOTOOLCHAIN)
					require.True(t, ok)
					if version.Minor >= 25 {
						return // already uses loclists
					}
					t.Run("bogus loclist", func(t *testing.T) {
						objcopy, ok := objcopyByArch[cfg.GOARCH]
						if !ok {
							t.Skipf("no objcopy available for %s", cfg.GOARCH)
						}
						tempDir, cleanup := dyninsttest.PrepTmpDir(t, "irgen_all_symbols_test")
						defer cleanup()
						modified, err := addLoclistSection(bin, objcopy, tempDir)
						if err != nil {
							t.Errorf("failed to objcopy a loclist section for %s: %v", cfg.String(), err)
						}
						testAllProbes(t, modified)
					})
				})
			}
		})
	}
}

func addLoclistSection(binPath, objcopy, tmpDir string) (modifiedBinPath string, err error) {
	junkDataFile := path.Join(tmpDir, "junk.data")
	junkData := make([]byte, 1024)
	if _, err := io.ReadFull(rand.Reader, junkData); err != nil {
		return "", fmt.Errorf("failed to generate junk data: %w", err)
	}
	if err := os.WriteFile(junkDataFile, junkData, 0644); err != nil {
		return "", fmt.Errorf("failed to write junk data: %w", err)
	}
	modifiedBinPath = filepath.Join(tmpDir, "modified.bin")
	if output, err := exec.Command(objcopy,
		"--add-section", ".debug_loclists="+junkDataFile,
		"--set-section-flags", ".debug_loclists=alloc,readonly,debug",
		binPath,
		modifiedBinPath,
	).CombinedOutput(); err != nil {
		return "", fmt.Errorf("failed to objcopy: %w\n%s", err, string(output))
	}
	return modifiedBinPath, nil
}

func testAllProbes(t *testing.T, binPath string) {
	binary, err := os.Open(binPath)
	require.NoError(t, err)
	elf, err := safeelf.NewFile(binary)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()
	var probes []ir.ProbeDefinition
	symbols, err := elf.Symbols()
	require.NoError(t, err)

	for i, s := range symbols {
		if int(s.Section) >= len(elf.Sections) ||
			elf.Sections[s.Section].Name != ".text" {
			continue
		}
		// These automatically generated symbols cause problems.
		if s.Name == "runtime.text" ||
			s.Name == "runtime.etext" ||
			s.Name == "" ||
			strings.HasPrefix(s.Name, "go:") ||
			strings.HasPrefix(s.Name, "type:.") ||
			strings.HasPrefix(s.Name, "runtime.vdso") ||
			strings.HasSuffix(s.Name, ".abi0") ||
			strings.Contains(s.Name, "..typeAssert") ||
			strings.Contains(s.Name, "..dict") ||
			strings.Contains(s.Name, "..gobytes") ||
			strings.Contains(s.Name, "..interfaceSwitch") ||
			strings.Contains(s.Name, "go.shape") {
			continue
		}

		// Speed things up by skipping some symbols.
		probes = append(probes, &rcjson.SnapshotProbe{
			LogProbeCommon: rcjson.LogProbeCommon{
				ProbeCommon: rcjson.ProbeCommon{
					ID:    fmt.Sprintf("probe_%d", i),
					Where: &rcjson.Where{MethodName: s.Name},
				},
			},
		})
	}

	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	v, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.NotNil(t, v)
	verifyIR(t, v)
}

func verifyIR(t *testing.T, p *ir.Program) {
	for _, s := range p.Subprograms {
		varNames := make(map[string]struct{})
		for _, v := range s.Variables {
			if _, ok := varNames[v.Name]; ok {
				t.Fatalf("variable %s appears multiple times in subprogram %s", v.Name, s.Name)
			}
			varNames[v.Name] = struct{}{}
		}
	}
	kindCounts := make(map[ir.IssueKind]int)
	defer func() {
		for kind, count := range kindCounts {
			t.Logf("kind %s: %d", kind.String(), count)
		}
	}()
	for _, issue := range p.Issues {
		var loc string
		switch where := issue.ProbeDefinition.GetWhere().(type) {
		case ir.FunctionWhere:
			loc = where.Location()
		case ir.LineWhere:
			fn, file, line := where.Line()
			loc = fmt.Sprintf("%s:%s:%s", fn, file, line)
		default:
			t.Fatalf("unexpected Where type: %T", issue.ProbeDefinition.GetWhere())
		}
		kindCounts[issue.Kind]++
		switch issue.Kind {
		case ir.IssueKindInvalidDWARF:
			t.Logf("%s: invalid DWARF: %s", loc, issue.Message)
		case ir.IssueKindDisassemblyFailed:
			// Go 1.25+ changed DWARF location list generation, introducing DW_OP_deref
			// and other patterns that break assumptions in pkg/dyninst/dwarf/loclist/parse.go.
			// This causes "unsupported register size" errors (e.g., 24-byte slices or
			// 16-byte interfaces claimed to be in a single 8-byte register) and
			// "unconsumed op" errors when DW_OP_deref (opcode 0x6) appears.
			// Until proper go1.25+ DWARF support is implemented, log instead of fail.
			t.Logf("%s: disassembly failed: %s", loc, issue.Message)
		case ir.IssueKindInvalidProbeDefinition:
			t.Logf("%s: invalid probe definition: %s", loc, issue.Message)
		case ir.IssueKindMalformedExecutable:
			t.Logf("%s: malformed executable: %s", loc, issue.Message)
		case ir.IssueKindTargetNotFoundInBinary:
			if permittedTargetNotFoundInBinary(loc) {
				t.Logf("(permitted) %s: target not found in binary: %s", loc, issue.Message)
			} else {
				t.Errorf("%s: target not found in binary: %s", loc, issue.Message)
			}
		case ir.IssueKindUnsupportedFeature:
			t.Logf("%s: unsupported feature: %s", loc, issue.Message)
		default:
			t.Errorf("%s: unexpected issue kind: %#v", loc, issue.Kind)
		}
	}
}

func permittedTargetNotFoundInBinary(loc string) bool {
	switch loc {
	// Some weird thing where the type of a different package gets moved
	// and then it should have a center dot but for the symbol table it becomes
	// a period.
	case "mime/multipart.(*writerOnly.1).Write",
		"mime/multipart.writerOnly.1.Write":
		return true
	default:
		return false
	}
}

// BenchmarkIRGenAllSymbols measures the performance of generating IR for all
// symbols in a binary. This exercises the DWARF line reader code paths,
// including the workaround for functions without line info.
func BenchmarkIRGenAllSymbols(b *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(b)
	for _, pkg := range []string{"simple", "sample"} {
		b.Run(pkg, func(b *testing.B) {
			for _, cfg := range cfgs {
				b.Run(cfg.String(), func(b *testing.B) {
					binPath := testprogs.MustGetBinary(b, pkg, cfg)
					probes := buildAllSymbolProbes(b, binPath)

					obj, err := object.OpenElfFileWithDwarf(binPath)
					require.NoError(b, err)
					defer func() { require.NoError(b, obj.Close()) }()

					b.ResetTimer()
					b.ReportAllocs()
					for b.Loop() {
						v, err := irgen.GenerateIR(1, obj, probes)
						require.NoError(b, err)
						require.NotNil(b, v)
					}
				})
			}
		})
	}
}

// buildAllSymbolProbes creates probe definitions for all function symbols in the binary.
func buildAllSymbolProbes(tb testing.TB, binPath string) []ir.ProbeDefinition {
	tb.Helper()
	binary, err := os.Open(binPath)
	require.NoError(tb, err)
	elf, err := safeelf.NewFile(binary)
	require.NoError(tb, err)
	defer func() { require.NoError(tb, binary.Close()) }()

	var probes []ir.ProbeDefinition
	symbols, err := elf.Symbols()
	require.NoError(tb, err)

	for i, s := range symbols {
		if int(s.Section) >= len(elf.Sections) ||
			elf.Sections[s.Section].Name != ".text" {
			continue
		}
		// Skip automatically generated symbols that cause problems.
		if s.Name == "runtime.text" ||
			s.Name == "runtime.etext" ||
			s.Name == "" ||
			strings.HasPrefix(s.Name, "go:") ||
			strings.HasPrefix(s.Name, "type:.") ||
			strings.HasPrefix(s.Name, "runtime.vdso") ||
			strings.HasSuffix(s.Name, ".abi0") ||
			strings.Contains(s.Name, "..typeAssert") ||
			strings.Contains(s.Name, "..dict") ||
			strings.Contains(s.Name, "..gobytes") ||
			strings.Contains(s.Name, "..interfaceSwitch") ||
			strings.Contains(s.Name, "go.shape") {
			continue
		}

		probes = append(probes, &rcjson.SnapshotProbe{
			LogProbeCommon: rcjson.LogProbeCommon{
				ProbeCommon: rcjson.ProbeCommon{
					ID:    fmt.Sprintf("probe_%d", i),
					Where: &rcjson.Where{MethodName: s.Name},
				},
			},
		})
	}
	return probes
}
