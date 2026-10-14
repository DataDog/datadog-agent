// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package usdt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	noteSection = ".note.stapsdt"
	baseSection = ".stapsdt.base"
	noteOwner   = "stapsdt"
	noteType    = 3
)

// Probe is a USDT probe site of an ELF file.
type Probe struct {
	// Provider is the name of the provider of the probe, e.g. "hotspot".
	Provider string
	// Name is the name of the probe, e.g. "gc__begin".
	Name string
	// Args is the raw argument specification of the site.
	Args string
	// Location is the virtual address of the site, adjusted for prelinking.
	Location uint64
	// FileOffset is the offset of the site in the file: the uprobe offset.
	FileOffset uint64
	// Semaphore is the virtual address of the semaphore of the probe, adjusted
	// for prelinking, or 0 if the probe has no semaphore.
	Semaphore uint64
	// SemaphoreFileOffset is the offset of the semaphore in the file (the
	// uprobe reference counter offset), or 0 if the probe has no semaphore.
	SemaphoreFileOffset uint64
	// Spec is the parsed argument specification of the site.
	Spec Spec
	// Err is not nil if the site cannot be attached: malformed note, site
	// outside an executable segment or not on a nop instruction, unsupported
	// argument. The other fields are then only informative.
	Err error
}

// ReadProbes returns all the USDT probe sites of f.
//
// A file without a .note.stapsdt section has no probe sites: ReadProbes
// returns an empty list and no error. An error is returned only if the notes
// cannot be enumerated; problems specific to a site are reported in its
// Probe.Err, so that the other sites remain usable.
func ReadProbes(f *safeelf.File) ([]Probe, error) {
	sec := f.Section(noteSection)
	if sec == nil {
		return nil, nil
	}
	if sec.Type != safeelf.SHT_NOTE {
		return nil, fmt.Errorf("section %s has type %s, expected %s", noteSection, sec.Type, safeelf.SHT_NOTE)
	}
	var addrSize int
	switch f.Class {
	case safeelf.ELFCLASS32:
		addrSize = 4
	case safeelf.ELFCLASS64:
		addrSize = 8
	default:
		return nil, fmt.Errorf("unsupported ELF class %s", f.Class)
	}
	data, err := sec.Data()
	if err != nil {
		return nil, fmt.Errorf("reading section %s: %w", noteSection, err)
	}
	// notes are 4-byte aligned, unless the section says otherwise (as libelf)
	align := uint64(4)
	if sec.Addralign == 8 {
		align = 8
	}
	notes, err := splitNotes(data, f.ByteOrder, align)
	if err != nil {
		return nil, fmt.Errorf("section %s: %w", noteSection, err)
	}

	// Prelinking moves the sections of a file after the notes have been
	// written: a note has the address of .stapsdt.base at build time, the
	// section header its current address.
	var baseAddr uint64
	if base := f.Section(baseSection); base != nil {
		baseAddr = base.Addr
	}

	var probes []Probe
	for _, n := range notes {
		if n.typ != noteType || string(n.name) != noteOwner {
			continue
		}
		note, err := parseNoteDesc(n.desc, addrSize, f.ByteOrder)
		if err != nil {
			probes = append(probes, Probe{Err: err})
			continue
		}
		probes = append(probes, newProbe(f, note, baseAddr))
	}
	return probes, nil
}

type rawNote struct {
	name []byte // without the trailing NULs
	typ  uint32
	desc []byte
}

// splitNotes splits the content of a SHT_NOTE section into its notes: a
// header (name size, descriptor size, type), the name and the descriptor,
// each padded to align bytes.
func splitNotes(data []byte, order binary.ByteOrder, align uint64) ([]rawNote, error) {
	var notes []rawNote
	for len(data) > 0 {
		if len(data) < 12 {
			return nil, errors.New("truncated note header")
		}
		nameSize := uint64(order.Uint32(data[0:4]))
		descSize := uint64(order.Uint32(data[4:8]))
		typ := order.Uint32(data[8:12])
		descStart := alignUp(12+nameSize, align)
		descEnd := descStart + descSize
		if descEnd > uint64(len(data)) {
			return nil, errors.New("truncated note")
		}
		notes = append(notes, rawNote{
			name: bytes.TrimRight(data[12:12+nameSize], "\x00"),
			typ:  typ,
			desc: data[descStart:descEnd],
		})
		// the padding of the last note may be missing
		data = data[min(alignUp(descEnd, align), uint64(len(data))):]
	}
	return notes, nil
}

func alignUp(v, align uint64) uint64 {
	return (v + align - 1) &^ (align - 1)
}

type sdtNote struct {
	location, base, semaphore uint64
	provider, name, args      string
}

// parseNoteDesc parses the descriptor of a stapsdt note: the addresses of the
// site, of .stapsdt.base and of the semaphore, followed by the provider name,
// the probe name and the argument specification, as NUL-terminated strings.
func parseNoteDesc(desc []byte, addrSize int, order binary.ByteOrder) (sdtNote, error) {
	if len(desc) < 3*addrSize {
		return sdtNote{}, errors.New("truncated stapsdt note")
	}
	readAddr := func(i int) uint64 {
		if addrSize == 4 {
			return uint64(order.Uint32(desc[i*4:]))
		}
		return order.Uint64(desc[i*8:])
	}
	strs := bytes.SplitN(desc[3*addrSize:], []byte{0}, 4)
	if len(strs) < 4 || len(strs[0]) == 0 || len(strs[1]) == 0 {
		return sdtNote{}, errors.New("malformed stapsdt note strings")
	}
	return sdtNote{
		location:  readAddr(0),
		base:      readAddr(1),
		semaphore: readAddr(2),
		provider:  string(strs[0]),
		name:      string(strs[1]),
		args:      string(strs[2]),
	}, nil
}

func newProbe(f *safeelf.File, note sdtNote, baseAddr uint64) Probe {
	p := Probe{
		Provider:  note.provider,
		Name:      note.name,
		Args:      note.args,
		Location:  note.location,
		Semaphore: note.semaphore,
	}
	if baseAddr != 0 && note.base != 0 {
		// the wrapping arithmetic handles both directions
		p.Location += baseAddr - note.base
		if p.Semaphore != 0 {
			p.Semaphore += baseAddr - note.base
		}
	}

	wrap := func(err error) error {
		return fmt.Errorf("probe %s:%s at %#x: %w", p.Provider, p.Name, p.Location, err)
	}

	off, err := siteFileOffset(f, p.Location)
	if err != nil {
		p.Err = wrap(err)
		return p
	}
	p.FileOffset = off

	if p.Semaphore != 0 {
		prog := findSegment(f, p.Semaphore, false)
		if prog == nil {
			p.Err = wrap(fmt.Errorf("semaphore %#x is not in a loadable segment", p.Semaphore))
			return p
		}
		p.SemaphoreFileOffset = p.Semaphore - prog.Vaddr + prog.Off
	}

	if p.Spec, err = ParseArgs(f.Machine, p.Args); err != nil {
		p.Err = wrap(err)
	}
	return p
}

// nop instructions that a probe site can be on, by architecture
var nops = map[safeelf.Machine][][]byte{
	// sys/sdt.h emits the one-byte nop
	safeelf.EM_X86_64:  {{0x90}, {0x66, 0x90}, {0x0f, 0x1f}, {0x66, 0x0f, 0x1f}},
	safeelf.EM_AARCH64: {{0x1f, 0x20, 0x03, 0xd5}},
}

// siteFileOffset returns the file offset of the probe site at the virtual
// address addr. As a uprobe replaces the instruction at that offset with a
// breakpoint, a wrong offset could corrupt the traced program: the site must
// be in an executable segment, on a nop instruction if the architecture is
// known.
func siteFileOffset(f *safeelf.File, addr uint64) (uint64, error) {
	prog := findSegment(f, addr, true)
	if prog == nil {
		return 0, errors.New("site is not in an executable segment")
	}
	if archNops, ok := nops[f.Machine]; ok {
		insn := make([]byte, 4)
		n, _ := prog.ReadAt(insn, int64(addr-prog.Vaddr))
		if !slices.ContainsFunc(archNops, func(nop []byte) bool { return bytes.HasPrefix(insn[:n], nop) }) {
			return 0, fmt.Errorf("site is not on a nop instruction (% x)", insn[:n])
		}
	}
	return addr - prog.Vaddr + prog.Off, nil
}

// findSegment returns the PT_LOAD segment, executable if exec is set, whose
// file content contains the virtual address addr, or nil.
func findSegment(f *safeelf.File, addr uint64, exec bool) *safeelf.Prog {
	for _, prog := range f.Progs {
		if prog.Type != safeelf.PT_LOAD || (exec && prog.Flags&safeelf.PF_X == 0) {
			continue
		}
		if addr >= prog.Vaddr && addr-prog.Vaddr < prog.Filesz {
			return prog
		}
	}
	return nil
}

// HostMachine returns the ELF machine of the running architecture, and
// whether this package supports it. Only attach uprobes to the probe sites of
// files targeting this machine.
func HostMachine() (safeelf.Machine, bool) {
	switch runtime.GOARCH {
	case "amd64":
		return safeelf.EM_X86_64, true
	case "arm64":
		return safeelf.EM_AARCH64, true
	default:
		return 0, false
	}
}
