// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package stackbudget computes, from a compiled eBPF object, the worst-case
// combined stack usage the kernel verifier will charge to a program.
//
// The verifier sums each subprogram's frame (rounded up to 32 bytes) along
// every call chain and rejects the program past 512 bytes. Frame sizes are
// compiler output, so this is computed offline without loading the program,
// which lets an x86 host check the architecture-sensitive arm64 budget.
package stackbudget

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// MaxStack is the kernel's MAX_BPF_STACK: the combined-stack ceiling.
const MaxStack = 512

// FrameAlign is the granularity the verifier rounds each frame up to.
const FrameAlign = 32

// MaxCallDepth is the kernel's MAX_CALL_FRAMES.
const MaxCallDepth = 8

// Chain is one call chain and the combined stack it costs.
type Chain struct {
	Path   []string // the chain, starting at a program entry point
	Frames []int    // rounded frame size charged for each Path entry
	Total  int      // sum of Frames, compared against MaxStack
}

func (c Chain) String() string {
	parts := make([]string, 0, len(c.Path))
	for i, name := range c.Path {
		if i < len(c.Frames) {
			parts = append(parts, fmt.Sprintf("%s(%d)", name, c.Frames[i]))
			continue
		}
		parts = append(parts, name)
	}
	return fmt.Sprintf("%d = %s", c.Total, strings.Join(parts, " -> "))
}

// Report is the analysis of a single object file.
type Report struct {
	Frames map[string]int   // subprogram name -> raw (unrounded) stack usage
	Worst  map[string]Chain // entry point name -> most expensive chain from it
}

// WorstChain returns the single most expensive chain across all entry points.
func (r Report) WorstChain() Chain {
	var worst Chain
	names := make([]string, 0, len(r.Worst))
	for name := range r.Worst {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic tie-breaking
	for _, name := range names {
		if c := r.Worst[name]; c.Total > worst.Total {
			worst = c
		}
	}
	return worst
}

// Round rounds a raw frame size the way the verifier does.
func Round(raw int) int {
	return (raw + FrameAlign - 1) / FrameAlign * FrameAlign
}

type function struct {
	name    string
	section safeelf.SectionIndex
	start   uint64 // byte offset within its section
	end     uint64
	isEntry bool
}

const insnSize = 8

// Analyze reads an unlinked eBPF object and reports per-subprogram frame sizes
// plus the worst call chain from every program entry point.
func Analyze(path string) (Report, error) {
	f, err := safeelf.Open(path)
	if err != nil {
		return Report{}, err // already names the path
	}
	defer f.Close()

	syms, err := f.Symbols()
	if err != nil {
		return Report{}, fmt.Errorf("symbols: %w", err)
	}

	// Functions, and the section symbols needed to resolve callback addresses.
	var funcs []function
	sectionSyms := map[int]safeelf.SectionIndex{}
	for i, s := range syms {
		switch safeelf.ST_TYPE(s.Info) {
		case safeelf.STT_FUNC:
			if s.Size == 0 {
				continue
			}
			// A subprogram lives in .text; a program entry point lives in its
			// own SEC() section (uprobe, kprobe, ...).
			sec := f.Sections[s.Section]
			funcs = append(funcs, function{
				name:    s.Name,
				section: s.Section,
				start:   s.Value,
				end:     s.Value + s.Size,
				isEntry: sec.Name != ".text",
			})
		case safeelf.STT_SECTION:
			sectionSyms[i] = s.Section
		}
	}
	if len(funcs) == 0 {
		return Report{}, fmt.Errorf("%s: no FUNC symbols", path)
	}

	at := func(sec safeelf.SectionIndex, off uint64) string {
		for _, fn := range funcs {
			if fn.section == sec && off >= fn.start && off < fn.end {
				return fn.name
			}
		}
		return ""
	}

	frames := map[string]int{}
	edges := map[string]map[string]bool{}
	for _, fn := range funcs {
		frames[fn.name] = 0
		edges[fn.name] = map[string]bool{}
	}

	// Relocations, indexed by (section, byte offset) so an instruction can be
	// matched to the symbol it references.
	type relKey struct {
		sec safeelf.SectionIndex
		off uint64
	}
	rels := map[relKey]safeelf.Symbol{}
	for _, sec := range f.Sections {
		if sec.Type != safeelf.SHT_REL || sec.Link == 0 {
			continue
		}
		target := safeelf.SectionIndex(sec.Info)
		data, err := sec.Data()
		if err != nil {
			return Report{}, fmt.Errorf("relocations of %s: %w", sec.Name, err)
		}
		for off := 0; off+16 <= len(data); off += 16 {
			r := binary.LittleEndian.Uint64(data[off:])
			info := binary.LittleEndian.Uint64(data[off+8:])
			symIdx := int(info >> 32)
			if symIdx == 0 || symIdx >= len(syms)+1 {
				continue
			}
			// Symbol indices in relocations are 1-based against the symtab.
			rels[relKey{target, r}] = syms[symIdx-1]
		}
	}

	// Decode each code section once.
	decoded := map[safeelf.SectionIndex][]byte{}
	for _, fn := range funcs {
		if _, ok := decoded[fn.section]; ok {
			continue
		}
		data, err := f.Sections[fn.section].Data()
		if err != nil {
			return Report{}, fmt.Errorf("section data: %w", err)
		}
		decoded[fn.section] = data
	}

	for sec, data := range decoded {
		for off := 0; off+insnSize <= len(data); off += insnSize {
			owner := at(sec, uint64(off))
			if owner == "" {
				continue
			}
			op := data[off]
			regs := data[off+1]
			dst := regs & 0x0f
			src := regs >> 4
			imm := int32(binary.LittleEndian.Uint32(data[off+4:]))
			offField := int16(binary.LittleEndian.Uint16(data[off+2:]))

			// Stack usage: a memory access based on r10 at a negative offset.
			// The frame must cover the lowest byte touched, so usage is -off.
			const (
				classLDX = 0x01
				classST  = 0x02
				classSTX = 0x03
				regFP    = 10
			)
			switch op & 0x07 {
			case classLDX:
				if src == regFP && offField < 0 && int(-offField) > frames[owner] {
					frames[owner] = int(-offField)
				}
			case classST, classSTX:
				if dst == regFP && offField < 0 && int(-offField) > frames[owner] {
					frames[owner] = int(-offField)
				}
			}

			// Calls to other subprograms carry a relocation to a FUNC symbol.
			const opCall = 0x85
			if op == opCall {
				if sym, ok := rels[relKey{sec, uint64(off)}]; ok {
					if safeelf.ST_TYPE(sym.Info) == safeelf.STT_FUNC && sym.Name != owner {
						edges[owner][sym.Name] = true
					}
				}
				continue
			}

			// lddw materialises callback addresses for helpers such as bpf_loop.
			// The verifier charges a callback's frame to the caller's chain, so
			// these are real edges. The relocation names the callee or its
			// section, with the byte offset in the immediate.
			const opLDDW = 0x18
			if op == opLDDW {
				if sym, ok := rels[relKey{sec, uint64(off)}]; ok {
					switch safeelf.ST_TYPE(sym.Info) {
					case safeelf.STT_FUNC:
						if sym.Name != owner {
							edges[owner][sym.Name] = true
						}
					case safeelf.STT_SECTION:
						if target := at(sym.Section, uint64(uint32(imm))); target != "" && target != owner {
							edges[owner][target] = true
						}
					}
				}
				off += insnSize // consume the second half
			}
		}
	}

	report := Report{Frames: frames, Worst: map[string]Chain{}}
	for _, fn := range funcs {
		if !fn.isEntry {
			continue
		}
		report.Worst[fn.name] = worstChain(fn.name, frames, edges)
	}
	if len(report.Worst) == 0 {
		return Report{}, fmt.Errorf("%s: no program entry point found", path)
	}
	return report, nil
}

func worstChain(entry string, frames map[string]int, edges map[string]map[string]bool) Chain {
	best := Chain{}
	path := []string{entry}
	seen := map[string]bool{entry: true}

	var walk func(node string, total int)
	walk = func(node string, total int) {
		if total > best.Total {
			best = Chain{Path: append([]string(nil), path...), Total: total}
		}
		if len(path) >= MaxCallDepth {
			return
		}
		// Deterministic order so equal-cost chains report identically.
		next := make([]string, 0, len(edges[node]))
		for callee := range edges[node] {
			next = append(next, callee)
		}
		sort.Strings(next)
		for _, callee := range next {
			if seen[callee] {
				continue // recursion is rejected by the verifier anyway
			}
			seen[callee] = true
			path = append(path, callee)
			walk(callee, total+Round(frames[callee]))
			path = path[:len(path)-1]
			delete(seen, callee)
		}
	}
	walk(entry, Round(frames[entry]))

	best.Frames = make([]int, 0, len(best.Path))
	for _, name := range best.Path {
		best.Frames = append(best.Frames, Round(frames[name]))
	}
	return best
}
