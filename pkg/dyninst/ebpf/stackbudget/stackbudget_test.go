// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stackbudget

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// budget is the kernel's own limit: a program over it fails to load. The
// dyninst program has historically sat right at it on arm64-debug, so we check
// it here rather than waiting for a KMT job to catch it.
const budget = MaxStack

// objects maps a variant name to the env var the build rule uses to hand over
// that variant's compiled object. Both architectures are checked from whichever
// host runs the test, since frame sizes are compiler output.
var objects = map[string]string{
	"arm64":        "DYNINST_EBPF_OBJ_ARM64",
	"arm64-debug":  "DYNINST_EBPF_OBJ_ARM64_DEBUG",
	"x86_64":       "DYNINST_EBPF_OBJ_X86_64",
	"x86_64-debug": "DYNINST_EBPF_OBJ_X86_64_DEBUG",
}

func TestCombinedStackBudget(t *testing.T) {
	checked := 0
	for variant, envVar := range objects {
		path := os.Getenv(envVar)
		if path == "" {
			continue
		}
		checked++
		t.Run(variant, func(t *testing.T) {
			resolved, err := locate(path)
			if err != nil {
				t.Fatalf("locating %s: %v", path, err)
			}
			report, err := Analyze(resolved)
			if err != nil {
				t.Fatalf("analyzing %s: %v", resolved, err)
			}
			for entry, chain := range report.Worst {
				t.Logf("%s: %s: worst chain %s (headroom %d bytes)",
					variant, entry, chain, budget-chain.Total)
			}
			worst := report.WorstChain()
			if worst.Total > budget {
				t.Errorf("combined stack of %d bytes exceeds the %d-byte budget; "+
					"the verifier will refuse to load this program.\nworst chain: %s\n"+
					"Shrink a frame on this chain to fix it.",
					worst.Total, budget, worst)
			}
		})
	}
	if checked == 0 {
		t.Skip("no eBPF objects provided; run via Bazel so the cross-architecture " +
			"objects are built and passed through the environment")
	}
}

// TestFrameSizingAssumption guards the one approximation Analyze makes about
// frame sizes: it charges only stack touched directly through r10, not stack
// reached through a derived pointer (r1 = r10; r1 += -N; a helper writes through
// r1). That holds only while every function's deepest stack access is a direct
// r10 access. If a future change makes a function reach deeper through a derived
// pointer, the analyzer would under-report its frame and could pass an
// over-budget program; this test fails first so the gap is fixed deliberately.
func TestFrameSizingAssumption(t *testing.T) {
	checked := 0
	for variant, envVar := range objects {
		path := os.Getenv(envVar)
		if path == "" {
			continue
		}
		checked++
		t.Run(variant, func(t *testing.T) {
			resolved, err := locate(path)
			if err != nil {
				t.Fatalf("locating %s: %v", path, err)
			}
			direct, derived := stackAccessDepths(t, resolved)
			for name, d := range derived {
				if d > direct[name] {
					t.Errorf("%s: %s reaches stack at -%d through a derived pointer but "+
						"only -%d directly; Analyze would under-report its frame. "+
						"Teach Analyze to track derived stack pointers.",
						variant, name, d, direct[name])
				}
			}
		})
	}
	if checked == 0 {
		t.Skip("no eBPF objects provided; run via Bazel")
	}
}

// stackAccessDepths returns, per function, the deepest negative r10 offset
// reached directly and the deepest reached through a pointer derived from r10
// (r1 = r10; r1 += -N). Both are positive byte depths.
func stackAccessDepths(t *testing.T, path string) (direct, derived map[string]int) {
	f, err := safeelf.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		t.Fatal(err)
	}
	var funcs []function
	for _, s := range syms {
		if safeelf.ST_TYPE(s.Info) == safeelf.STT_FUNC && s.Size > 0 {
			funcs = append(funcs, function{
				name:    s.Name,
				section: s.Section,
				start:   s.Value,
				end:     s.Value + s.Size,
			})
		}
	}
	at := func(sec safeelf.SectionIndex, off uint64) string {
		for _, fn := range funcs {
			if fn.section == sec && off >= fn.start && off < fn.end {
				return fn.name
			}
		}
		return ""
	}
	decoded := map[safeelf.SectionIndex][]byte{}
	for _, fn := range funcs {
		if _, ok := decoded[fn.section]; ok {
			continue
		}
		data, err := f.Sections[fn.section].Data()
		if err != nil {
			t.Fatal(err)
		}
		decoded[fn.section] = data
	}
	direct = map[string]int{}
	derived = map[string]int{}
	for sec, data := range decoded {
		// base[r] holds whether register r currently aliases r10, and at what
		// offset. Tracking is reset at each function boundary.
		var base [11]bool
		var baseOff [11]int
		owner := ""
		for off := 0; off+insnSize <= len(data); off += insnSize {
			if o := at(sec, uint64(off)); o != owner {
				owner = o
				base = [11]bool{}
				baseOff = [11]int{}
			}
			if owner == "" {
				continue
			}
			op := data[off]
			regs := data[off+1]
			dst := regs & 0x0f
			src := regs >> 4
			imm := int32(binary.LittleEndian.Uint32(data[off+4:]))
			offField := int16(binary.LittleEndian.Uint16(data[off+2:]))
			const (
				classLDX = 0x01
				classST  = 0x02
				classSTX = 0x03
				regFP    = 10
				opMovReg = 0xbf
				opAddImm = 0x07
				opLDDW   = 0x18
			)
			switch op & 0x07 {
			case classLDX:
				if src == regFP && offField < 0 && int(-offField) > direct[owner] {
					direct[owner] = int(-offField)
				}
			case classST, classSTX:
				if dst == regFP && offField < 0 && int(-offField) > direct[owner] {
					direct[owner] = int(-offField)
				}
			}
			switch op {
			case opMovReg:
				if dst < regFP {
					if src == regFP {
						base[dst], baseOff[dst] = true, 0
					} else if src < regFP && base[src] {
						base[dst], baseOff[dst] = true, baseOff[src]
					} else {
						base[dst] = false
					}
				}
			case opAddImm:
				if dst < regFP && base[dst] {
					baseOff[dst] += int(imm)
					if -baseOff[dst] > derived[owner] {
						derived[owner] = -baseOff[dst]
					}
				}
			case opLDDW:
				invalidateStackPointers(&base, op, dst)
				off += insnSize
			default:
				invalidateStackPointers(&base, op, dst)
			}
		}
	}
	return direct, derived
}

func invalidateStackPointers(base *[11]bool, op, dst byte) {
	if op == 0x85 { // Calls overwrite r0 through r5.
		for r := 0; r <= 5; r++ {
			base[r] = false
		}
		return
	}
	switch op & 0x07 {
	case 0x00, 0x01, 0x04, 0x07: // Loads and arithmetic overwrite their destination.
		if dst < 10 {
			base[dst] = false
		}
	}
}

func TestInvalidateStackPointers(t *testing.T) {
	for _, tc := range []struct {
		name string
		op   byte
		want bool
	}{
		{"load immediate", 0x18, false},
		{"load memory", 0x79, false},
		{"move immediate", 0xb7, false},
		{"32-bit move", 0xbc, false},
		{"32-bit arithmetic", 0x04, false},
		{"64-bit arithmetic", 0x0f, false},
		{"store immediate", 0x7a, true},
		{"store register", 0x7b, true},
		{"conditional jump", 0x15, true},
		{"32-bit conditional jump", 0x16, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var base [11]bool
			base[2], base[6] = true, true
			invalidateStackPointers(&base, tc.op, 2)
			if base[2] != tc.want || !base[6] {
				t.Fatalf("pointer aliases: got %v, want r2=%t and r6=true", base, tc.want)
			}
		})
	}
	t.Run("call", func(t *testing.T) {
		var base [11]bool
		for r := range base {
			base[r] = true
		}
		invalidateStackPointers(&base, 0x85, 0)
		for r, alias := range base {
			if alias != (r > 5) {
				t.Errorf("r%d: got alias=%t, want %t", r, alias, r > 5)
			}
		}
	})
}

// locate resolves a data dependency's path: absolute as-is, otherwise try the
// Bazel runfiles tree before walking up from the working directory.
func locate(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}

	candidates := []string{path}
	if dir := os.Getenv("TEST_SRCDIR"); dir != "" {
		if ws := os.Getenv("TEST_WORKSPACE"); ws != "" {
			candidates = append(candidates, filepath.Join(dir, ws, path))
		}
		candidates = append(candidates, filepath.Join(dir, path))
	}
	// Fall back to walking up, which covers running the test directly.
	prefix := ""
	for i := 0; i < 8; i++ {
		prefix = filepath.Join(prefix, "..")
		candidates = append(candidates, filepath.Join(prefix, path))
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("not found in any of %v", candidates)
}
