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
	"slices"
	"strings"

	"github.com/cilium/ebpf/btf"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

const (
	btfExtMagic     = 0xeB9F
	btfExtHeaderLen = 32
	coreReloRecLen  = 16
	// Offset of str_len in struct btf_header.
	btfStrLenOff = 20
)

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
	rawBTF, err := sectionData(f, ".BTF")
	if err != nil {
		return nil, nil, err
	}
	rawExt, err := sectionData(f, ".BTF.ext")
	if err != nil {
		return nil, nil, err
	}
	// Raw .BTF skips the ELF symbol fixups of datasecs, which are never relocation roots.
	spec, err := btf.LoadSpecFromReader(bytes.NewReader(rawBTF))
	if err != nil {
		return nil, nil, fmt.Errorf("load BTF: %w", err)
	}
	strs, err := stringTable(rawBTF, order)
	if err != nil {
		return nil, nil, err
	}
	relos, err := coreRelos(rawExt, strs, spec, order)
	return relos, order, err
}

func sectionData(f *safeelf.File, name string) ([]byte, error) {
	s := f.Section(name)
	if s == nil {
		return nil, fmt.Errorf("no %s section", name)
	}
	return s.Data()
}

func stringTable(raw []byte, order binary.ByteOrder) ([]byte, error) {
	if len(raw) < 24 {
		return nil, errors.New(".BTF header truncated")
	}
	hdrLen := order.Uint32(raw[4:])
	strOff := order.Uint32(raw[16:])
	strLen := order.Uint32(raw[20:])
	start := uint64(hdrLen) + uint64(strOff)
	if start+uint64(strLen) > uint64(len(raw)) {
		return nil, errors.New(".BTF string table out of bounds")
	}
	return raw[start : start+uint64(strLen)], nil
}

func cString(strs []byte, off uint32) (string, error) {
	if uint64(off) >= uint64(len(strs)) {
		return "", fmt.Errorf("string offset %d out of bounds", off)
	}
	end := bytes.IndexByte(strs[off:], 0)
	if end < 0 {
		return "", fmt.Errorf("unterminated string at %d", off)
	}
	return string(strs[off : off+uint32(end)]), nil
}

func coreRelos(raw, strs []byte, spec *btf.Spec, order binary.ByteOrder) ([]relo, error) {
	if len(raw) < 24 || order.Uint16(raw) != btfExtMagic {
		return nil, errors.New("bad .BTF.ext header")
	}
	hdrLen := order.Uint32(raw[4:])
	if hdrLen < btfExtHeaderLen {
		return nil, nil
	}
	if uint64(len(raw)) < uint64(hdrLen) {
		return nil, errors.New(".BTF.ext header truncated")
	}
	off, size := order.Uint32(raw[24:]), order.Uint32(raw[28:])
	if size == 0 {
		return nil, nil
	}
	start := uint64(hdrLen) + uint64(off)
	if start+uint64(size) > uint64(len(raw)) || size < 4 {
		return nil, errors.New("core_relo info out of bounds")
	}
	sec := raw[start : start+uint64(size)]
	recLen := order.Uint32(sec)
	if recLen < coreReloRecLen {
		return nil, fmt.Errorf("core_relo record size %d too small", recLen)
	}

	var relos []relo
	for p := uint32(4); p < uint32(len(sec)); {
		if 8 > uint32(len(sec))-p {
			return nil, errors.New("core_relo section header truncated")
		}
		n := order.Uint32(sec[p+4:])
		p += 8
		for range n {
			if recLen > uint32(len(sec))-p {
				return nil, errors.New("core_relo record truncated")
			}
			typeID := btf.TypeID(order.Uint32(sec[p+4:]))
			access, err := cString(strs, order.Uint32(sec[p+8:]))
			if err != nil {
				return nil, err
			}
			typ, err := spec.TypeByID(typeID)
			if err != nil {
				return nil, fmt.Errorf("relocation type %d: %w", typeID, err)
			}
			relos = append(relos, relo{kind: order.Uint32(sec[p+12:]), typ: typ, access: access})
			p += recLen
		}
	}
	return relos, nil
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

	var reloInfo []byte
	if len(relos) > 0 {
		reloInfo = order.AppendUint32(reloInfo, coreReloRecLen)
		reloInfo = order.AppendUint32(reloInfo, addString("relocations"))
		reloInfo = order.AppendUint32(reloInfo, uint32(len(relos)))
		for i, r := range relos {
			reloInfo = order.AppendUint32(reloInfo, uint32(i*8))
			reloInfo = order.AppendUint32(reloInfo, uint32(ids[i]))
			reloInfo = order.AppendUint32(reloInfo, addString(r.access))
			reloInfo = order.AppendUint32(reloInfo, r.kind)
		}
	}

	ext := make([]byte, 0, btfExtHeaderLen+len(reloInfo))
	ext = order.AppendUint16(ext, btfExtMagic)
	ext = append(ext, 1, 0)
	ext = order.AppendUint32(ext, btfExtHeaderLen)
	for range 4 { // func_info and line_info: empty
		ext = order.AppendUint32(ext, 0)
	}
	ext = order.AppendUint32(ext, 0)
	ext = order.AppendUint32(ext, uint32(len(reloInfo)))
	ext = append(ext, reloInfo...)

	return writeELF(order, []elfSection{{".BTF", rawBTF}, {".BTF.ext", ext}})
}

type elfSection struct {
	name string
	data []byte
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
	shtStrtab    = 3
	elfHeaderLen = 64
	shdrLen      = 64
)

// writeELF writes a relocatable eBPF ELF with the given sections and a
// section name table, which is all libbpf's btf__parse() needs.
func writeELF(order byteOrder, secs []elfSection) ([]byte, error) {
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
		shdrs = append(shdrs, elfSectionHeader{
			Name:      nameOff(s.name),
			Type:      shtProgbits,
			Off:       off,
			Size:      uint64(len(s.data)),
			Addralign: 4,
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
