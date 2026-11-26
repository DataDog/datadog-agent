// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main writes a stub ELF holding only what `bpftool gen min_core_btf`
// reads from eBPF objects: their CO-RE relocations and the local BTF types they
// reference. Relocations are grouped by local root type and the groups of all
// objects are merged, deduplicated and sorted, so the stub does not change with
// code that leaves the relocations alone.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cilium/ebpf/btf"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	btfExtMagic     = 0xeB9F
	btfExtHeaderLen = 32
	funcInfoRecLen  = 8
	lineInfoRecLen  = 16
	coreReloRecLen  = 16
	// Offset of str_len in struct btf_header.
	btfStrLenOff = 20
)

// btf.CORERelocation keeps its fields unexported, so they are parsed from its
// String(). That format is not a stable API: anything unexpected is an error.
var reloPattern = regexp.MustCompile(`^CORERelocation\(([a-z0-9_]+), .*\[([0-9:]+)\], local_id=([0-9]+)\)$`)

// Names printed by cilium/ebpf for the kernel's enum bpf_core_relo_kind values.
var reloKinds = map[string]uint32{
	"byte_off":       0,
	"byte_sz":        1,
	"field_exists":   2,
	"signed":         3,
	"lshift_u64":     4,
	"rshift_u64":     5,
	"local_type_id":  6,
	"target_type_id": 7,
	"type_exists":    8,
	"type_size":      9,
	"enumval_exists": 10,
	"enumval_value":  11,
	"type_matches":   12,
}

// byteOrder is satisfied by binary.LittleEndian and binary.BigEndian.
type byteOrder interface {
	binary.ByteOrder
	binary.AppendByteOrder
}

type relo struct {
	kind   uint32
	typ    btf.Type
	access string
}

func main() {
	out := flag.String("o", "", "path of the stub to write")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: corerelo -o STUB OBJECT...")
		os.Exit(2)
	}
	if err := run(flag.Args(), *out); err != nil {
		fmt.Fprintf(os.Stderr, "corerelo: %v\n", err)
		os.Exit(1)
	}
}

func run(ins []string, out string) error {
	var relos []relo
	var order byteOrder
	// Identical groups from several objects are taken from the first one, so the
	// stub must not depend on the argument order.
	for _, in := range slices.Sorted(slices.Values(ins)) {
		r, o, err := readRelos(in)
		if err != nil {
			return fmt.Errorf("%s: %w", in, err)
		}
		if order != nil && o != order {
			return fmt.Errorf("%s: mixed byte orders", in)
		}
		relos, order = append(relos, r...), o
	}
	// libbpf rejects a .BTF.ext without records; such objects add nothing to minimization.
	if len(relos) == 0 {
		return os.WriteFile(out, nil, 0o644)
	}
	stub, err := buildStub(relos, order)
	if err != nil {
		return err
	}
	return os.WriteFile(out, stub, 0o644)
}

func readRelos(in string) ([]relo, byteOrder, error) {
	data, err := os.ReadFile(in)
	if err != nil {
		return nil, nil, err
	}
	f, err := safeelf.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	order, ok := f.ByteOrder.(byteOrder)
	if !ok {
		return nil, nil, errors.New("unsupported ELF byte order")
	}
	spec, ext, err := btf.LoadSpecAndExtInfosFromReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("load BTF: %w", err)
	}
	if ext == nil {
		return nil, nil, errors.New("no .BTF.ext section")
	}

	// bpftool evaluates relocations in .BTF.ext order, and that order can matter
	// when a kernel has several candidates for a root type. cilium/ebpf sorts
	// each section by instruction offset, which is how clang emits them, and
	// sections are taken in ELF order. minimized_btfs_test checks the result.
	var names []string
	for _, s := range f.Sections {
		if _, ok := ext.CORERelos[s.Name]; ok && !slices.Contains(names, s.Name) {
			names = append(names, s.Name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(ext.CORERelos)) {
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	var relos []relo
	for _, name := range names {
		for _, r := range ext.CORERelos[name] {
			rl, err := parseRelo(r.Relo, spec)
			if err != nil {
				return nil, nil, fmt.Errorf("section %s: %w", name, err)
			}
			relos = append(relos, rl)
		}
	}
	return relos, order, nil
}

func parseRelo(r *btf.CORERelocation, spec *btf.Spec) (relo, error) {
	m := reloPattern.FindStringSubmatch(r.String())
	if m == nil {
		return relo{}, fmt.Errorf("unexpected relocation format %q", r)
	}
	kind, ok := reloKinds[m[1]]
	if !ok {
		return relo{}, fmt.Errorf("unknown relocation kind %q", m[1])
	}
	id, err := strconv.ParseUint(m[3], 10, 32)
	if err != nil {
		return relo{}, fmt.Errorf("relocation type ID %q: %w", m[3], err)
	}
	typ, err := spec.TypeByID(btf.TypeID(id))
	if err != nil {
		return relo{}, fmt.Errorf("relocation type %d: %w", id, err)
	}
	return relo{kind: kind, typ: typ, access: m[2]}, nil
}

// typeKey identifies a local type by name and definition, so same-named types
// from different objects only collapse when they are identical.
func typeKey(t btf.Type) (string, error) {
	var b btf.Builder
	if _, err := b.Add(t); err != nil {
		return "", err
	}
	raw, err := b.Marshal(nil, &btf.MarshalOptions{Order: binary.LittleEndian})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%T %s %x", t, t.TypeName(), sha256.Sum256(raw)), nil
}

// groupRelos orders relocations the way libbpf evaluates them without changing
// the outcome. libbpf prunes the candidates of each local root type with every
// relocation against it, in order, so order only matters within a root. Roots
// stay distinct across objects, since merging two would chain their pruning.
func groupRelos(relos []relo) ([]relo, error) {
	var roots []btf.Type
	groups := map[btf.Type][]relo{}
	for _, r := range relos {
		g := groups[r.typ]
		if n := len(g); n > 0 && g[n-1].kind == r.kind && g[n-1].access == r.access {
			continue
		}
		if len(g) == 0 {
			roots = append(roots, r.typ)
		}
		groups[r.typ] = append(g, r)
	}

	byKey := make(map[string][]relo, len(roots))
	for _, root := range roots {
		tk, err := typeKey(root)
		if err != nil {
			return nil, err
		}
		var key strings.Builder
		key.WriteString(tk)
		for _, r := range groups[root] {
			fmt.Fprintf(&key, "\x00%d %s", r.kind, r.access)
		}
		// Identical roots with identical relocations select the same types.
		if _, ok := byKey[key.String()]; !ok {
			byKey[key.String()] = groups[root]
		}
	}
	var out []relo
	for _, key := range slices.Sorted(maps.Keys(byKey)) {
		out = append(out, byKey[key]...)
	}
	return out, nil
}

func buildStub(relos []relo, order byteOrder) ([]byte, error) {
	relos, err := groupRelos(relos)
	if err != nil {
		return nil, err
	}

	var b btf.Builder
	ids := make([]btf.TypeID, len(relos))
	for i, r := range relos {
		id, err := b.Add(r.typ)
		if err != nil {
			return nil, fmt.Errorf("add %s: %w", r.typ, err)
		}
		ids[i] = id
	}
	funcID, err := b.Add(&btf.Func{Name: "relocations", Type: &btf.FuncProto{Return: &btf.Void{}}})
	if err != nil {
		return nil, err
	}
	rawBTF, err := b.Marshal(nil, &btf.MarshalOptions{Order: order})
	if err != nil {
		return nil, fmt.Errorf("marshal BTF: %w", err)
	}

	strOffs := map[string]uint32{}
	addString := func(s string) uint32 {
		if off, ok := strOffs[s]; ok {
			return off
		}
		off := order.Uint32(rawBTF[btfStrLenOff:])
		rawBTF = append(append(rawBTF, s...), 0)
		order.PutUint32(rawBTF[btfStrLenOff:], off+uint32(len(s))+1)
		strOffs[s] = off
		return off
	}

	secName := addString("relocations")
	// extInfo starts a func_info, line_info or core_relo block with one section.
	extInfo := func(recLen uint32, n int) []byte {
		b := order.AppendUint32(nil, recLen)
		b = order.AppendUint32(b, secName)
		return order.AppendUint32(b, uint32(n))
	}
	// cilium/ebpf fails on empty func_info or line_info, and libbpf on a block
	// without records, so both get a placeholder record bpftool does not use.
	funcInfo := order.AppendUint32(extInfo(funcInfoRecLen, 1), 0)
	funcInfo = order.AppendUint32(funcInfo, uint32(funcID))
	lineInfo := extInfo(lineInfoRecLen, 1)
	for range 4 { // insn_off, file_name_off, line_off, line_col
		lineInfo = order.AppendUint32(lineInfo, 0)
	}
	reloInfo := extInfo(coreReloRecLen, len(relos))
	for i, r := range relos {
		reloInfo = order.AppendUint32(reloInfo, uint32(i*8))
		reloInfo = order.AppendUint32(reloInfo, uint32(ids[i]))
		reloInfo = order.AppendUint32(reloInfo, addString(r.access))
		reloInfo = order.AppendUint32(reloInfo, r.kind)
	}

	ext := order.AppendUint16(nil, btfExtMagic)
	ext = append(ext, 1, 0)
	ext = order.AppendUint32(ext, btfExtHeaderLen)
	off := uint32(0)
	for _, blk := range [][]byte{funcInfo, lineInfo, reloInfo} {
		ext = order.AppendUint32(ext, off)
		ext = order.AppendUint32(ext, uint32(len(blk)))
		off += uint32(len(blk))
	}
	ext = slices.Concat(ext, funcInfo, lineInfo, reloInfo)

	return writeELF(order, []elfSection{
		{name: ".BTF", typ: shtProgbits, data: rawBTF},
		{name: ".BTF.ext", typ: shtProgbits, data: ext},
		// cilium/ebpf only loads BTF from an ELF with a symbol table; this one
		// holds just the null symbol.
		{name: ".symtab", typ: shtSymtab, entsize: symLen, data: make([]byte, symLen)},
	})
}

type elfSection struct {
	name    string
	typ     uint32
	entsize uint64
	data    []byte
}

// Elf64_Ehdr and Elf64_Shdr, which pkg/util/safeelf does not expose for writing.
type elfHeader struct {
	Ident     [16]byte
	Type      uint16
	Machine   uint16
	Version   uint32
	Entry     uint64
	Phoff     uint64
	Shoff     uint64
	Flags     uint32
	Ehsize    uint16
	Phentsize uint16
	Phnum     uint16
	Shentsize uint16
	Shnum     uint16
	Shstrndx  uint16
}

type elfSectionHeader struct {
	Name      uint32
	Type      uint32
	Flags     uint64
	Addr      uint64
	Off       uint64
	Size      uint64
	Link      uint32
	Info      uint32
	Addralign uint64
	Entsize   uint64
}

const (
	elfClass64   = 2
	elfData2LSB  = 1
	elfData2MSB  = 2
	elfVersion   = 1
	elfTypeRel   = 1
	elfMachBPF   = 247
	shtProgbits  = 1
	shtSymtab    = 2
	shtStrtab    = 3
	elfHeaderLen = 64
	shdrLen      = 64
	symLen       = 24
)

// writeELF writes a relocatable eBPF ELF with the given sections and a
// section name table, which also serves as the string table of any symbol table.
func writeELF(order byteOrder, secs []elfSection) ([]byte, error) {
	shstrtabIdx := uint32(len(secs) + 1)
	shstrtab := []byte{0}
	nameOff := func(s string) uint32 {
		off := uint32(len(shstrtab))
		shstrtab = append(append(shstrtab, s...), 0)
		return off
	}
	var body []byte
	off := uint64(elfHeaderLen)
	align := func() {
		for off%8 != 0 {
			body = append(body, 0)
			off++
		}
	}

	shdrs := []elfSectionHeader{{}}
	for _, s := range secs {
		align()
		var link, info uint32
		if s.typ == shtSymtab {
			link, info = shstrtabIdx, 1
		}
		shdrs = append(shdrs, elfSectionHeader{
			Name:      nameOff(s.name),
			Type:      s.typ,
			Off:       off,
			Size:      uint64(len(s.data)),
			Link:      link,
			Info:      info,
			Addralign: 8,
			Entsize:   s.entsize,
		})
		body = append(body, s.data...)
		off += uint64(len(s.data))
	}
	shstrtabName := nameOff(".shstrtab")
	shdrs = append(shdrs, elfSectionHeader{
		Name:      shstrtabName,
		Type:      shtStrtab,
		Off:       off,
		Size:      uint64(len(shstrtab)),
		Addralign: 1,
	})
	body = append(body, shstrtab...)
	off += uint64(len(shstrtab))
	align()

	hdr := elfHeader{
		Type:      elfTypeRel,
		Machine:   elfMachBPF,
		Version:   elfVersion,
		Shoff:     off,
		Ehsize:    elfHeaderLen,
		Shentsize: shdrLen,
		Shnum:     uint16(len(shdrs)),
		Shstrndx:  uint16(len(shdrs) - 1),
	}
	copy(hdr.Ident[:], "\x7fELF")
	hdr.Ident[4] = elfClass64
	hdr.Ident[5] = elfData2LSB
	if order.String() == binary.BigEndian.String() {
		hdr.Ident[5] = elfData2MSB
	}
	hdr.Ident[6] = elfVersion

	var buf bytes.Buffer
	if err := binary.Write(&buf, order, hdr); err != nil {
		return nil, err
	}
	buf.Write(body)
	if err := binary.Write(&buf, order, shdrs); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
