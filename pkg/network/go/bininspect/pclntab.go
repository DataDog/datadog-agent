package bininspect

import (
	"debug/elf"
	"encoding/binary"
	"strings"
	"sync"
)

func pclnFuncs(f *elf.File) []Func {
	pclntabSect := f.Section(".gopclntab")
	if pclntabSect == nil {
		return nil
	}

	textSect := f.Section(".text")
	if textSect == nil {
		return nil
	}

	return getfuncs(newLineTable(pclntabSect, textSect.Addr))
}

// A Func collects information about a single function.
type Func struct {
	Entry uint64
	End   uint64
	Name  string
}

func getfuncs(pcln *lineTable) []Func {
	if pcln.isGo12("") {
		return pcln.go12Funcs()
	}
	return nil
}

// version of the pclntab
type version int

const (
	verUnknown version = iota
	ver11
	ver12
	ver116
	ver118
	ver120
)

type sectionAccess struct {
	sect *elf.Section
	off  int64
}

func (s *sectionAccess) ReadAt(p []byte, off int64) (n int, err error) {
	return s.sect.ReadAt(p, s.off+off)
}

type lineTable struct {
	Section *elf.Section
	PC      uint64

	// This mutex is used to keep parsing of pclntab synchronous.
	mu sync.Mutex

	// Contains the version of the pclntab section.
	Version version

	// Go 1.16/1.18 state
	Binary      binary.ByteOrder
	Ptrsize     uint32
	textStart   uint64 // address of runtime.text symbol (1.18+)
	funcnametab sectionAccess
	funcdata    sectionAccess
	functab     sectionAccess
	nfunctab    uint32

	funcNameHelper      []byte
	ptrBufferSizeHelper []byte
}

// newLineTable returns a new PC/line table
// corresponding to the encoded data.
// Text must be the start address of the
// corresponding text segment.
func newLineTable(sect *elf.Section, text uint64) *lineTable {
	return &lineTable{Section: sect, PC: text, funcNameHelper: make([]byte, 25)}
}

// Go 1.2 symbol table format.
// See golang.org/s/go12symtab.
//
// A general note about the methods here: rather than try to avoid
// index out of bounds errors, we trust Go to detect them, and then
// we recover from the panics and treat them as indicative of a malformed
// or incomplete table.
//
// The methods called by symtab.go, which begin with "go12" prefixes,
// are expected to have that recovery logic.

// isGo12 reports whether this is a Go 1.2 (or later) symbol table.
func (t *lineTable) isGo12(versionOverride string) bool {
	t.parsePclnTab(versionOverride)
	return t.Version >= ver12
}

const (
	go116magic = 0xfffffffa
	go118magic = 0xfffffff0
	go120magic = 0xfffffff1
)

// uintptr returns the pointer-sized value encoded at b.
// The pointer size is dictated by the table being read.
func (t *lineTable) uintptr(b []byte) uint64 {
	if t.Ptrsize == 4 {
		return uint64(t.Binary.Uint32(b))
	}
	return t.Binary.Uint64(b)
}

// parsePclnTab parses the pclntab, setting the version.
func (t *lineTable) parsePclnTab(versionOverride string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.Version != verUnknown {
		return
	}

	// Note that during this function, setting the version is the last thing we do.
	// If we set the version too early, and parsing failed (likely as a panic on
	// slice lookups), we'd have a mistaken version.
	//
	// Error paths through this code will default the version to 1.1.
	t.Version = ver11

	local := make([]byte, 8)
	if n, err := t.Section.ReadAt(local, 0); err != nil || n != 8 {
		return
	}
	// Check header: 4-byte magic, two zeros, pc quantum, pointer size.
	if t.Section.Size < 16 || local[4] != 0 || local[5] != 0 ||
		(local[6] != 1 && local[6] != 2 && local[6] != 4) || // pc quantum
		(local[7] != 4 && local[7] != 8) { // pointer size
		return
	}

	var possibleVersion version
	leMagic := binary.LittleEndian.Uint32(local)
	beMagic := binary.BigEndian.Uint32(local)
	switch {
	case leMagic == go116magic:
		t.Binary, possibleVersion = binary.LittleEndian, ver116
	case beMagic == go116magic:
		t.Binary, possibleVersion = binary.BigEndian, ver116
	case leMagic == go118magic:
		t.Binary, possibleVersion = binary.LittleEndian, ver118
	case beMagic == go118magic:
		t.Binary, possibleVersion = binary.BigEndian, ver118
	case leMagic == go120magic:
		t.Binary, possibleVersion = binary.LittleEndian, ver120
	case beMagic == go120magic:
		t.Binary, possibleVersion = binary.BigEndian, ver120
	default:
		return
	}
	t.Version = possibleVersion

	if len(versionOverride) > 0 {
		if strings.Contains(versionOverride, "1.20") {
			t.Version = ver120
		} else if strings.Contains(versionOverride, "1.19") {
			t.Version = ver118
		} else if strings.Contains(versionOverride, "1.18") {
			t.Version = ver118
		} else if strings.Contains(versionOverride, "1.17") {
			t.Version = ver116
		} else if strings.Contains(versionOverride, "1.16") {
			t.Version = ver116
		} else {
			return
		}
	}

	// quantum and ptrSize are the same between 1.2, 1.16, and 1.18
	t.Ptrsize = uint32(local[7])
	t.ptrBufferSizeHelper = make([]byte, t.Ptrsize)

	offset := func(word uint32) uint64 {
		off := 8 + word*t.Ptrsize
		if n, err := t.Section.ReadAt(t.ptrBufferSizeHelper, int64(off)); err != nil || n != int(t.Ptrsize) {
			return 0
		}
		b := t.uintptr(t.ptrBufferSizeHelper)
		return b
	}

	switch t.Version {
	case ver118, ver120:
		t.nfunctab = uint32(offset(0))
		t.textStart = t.PC // use the start PC instead of reading from the table, which may be unrelocated
		t.funcnametab = sectionAccess{
			sect: t.Section,
			off:  int64(offset(3)),
		}
		t.funcdata = sectionAccess{
			sect: t.Section,
			off:  int64(offset(7)),
		}
		t.functab = sectionAccess{
			sect: t.Section,
			off:  int64(offset(7)),
		}
	case ver116:
		t.nfunctab = uint32(offset(0))
		t.funcnametab = sectionAccess{
			sect: t.Section,
			off:  int64(offset(2)),
		}
		t.funcdata = sectionAccess{
			sect: t.Section,
			off:  int64(offset(6)),
		}
		t.functab = sectionAccess{
			sect: t.Section,
			off:  int64(offset(6)),
		}
	default:
		panic("unreachable")
	}
}

var (
	functions = map[string]struct{}{
		"crypto/tls.(*Conn).Read":  {},
		"crypto/tls.(*Conn).Write": {},
		"crypto/tls.(*Conn).Close": {},
	}
)

func isRelevantFunction(pkg string) bool {
	_, ok := functions[pkg]
	return ok
}

const chunk = 10

// go12Funcs returns a slice of Funcs derived from the Go 1.2+ pcln table.
func (t *lineTable) go12Funcs() []Func {
	// avoid OOM error on corrupt binaries
	// empirically gathered. Most binaries are <= UINT16_MAX, but some truly huge have >= 100000 functions
	ft := t.funcTab(chunk)

	funcs := make([]Func, len(functions))
	found := 0
	helper := make([]byte, t.Ptrsize)
	data := sectionAccess{
		sect: t.Section,
	}
	nextIdx := 0
	for currentIdx := 0; currentIdx <= ft.Count(); currentIdx++ {
		if currentIdx%chunk == 0 {
			nextChunk := chunk
			if currentIdx+nextChunk > ft.Count() {
				nextChunk = ft.Count() - currentIdx
			}
			ft.loadCache(currentIdx, nextChunk)
			nextIdx = 0
		}

		data.off = int64(ft.funcOffFromCache(nextIdx)) + t.funcdata.off
		nameoff := field(t.Ptrsize, t.Version, t.Binary, data, helper)
		funcName := t.funcName(nameoff)

		nextIdx++
		if funcName == "" {
			continue
		}
		index := found
		found++
		f := &funcs[index]
		f.Entry = ft.pc(currentIdx)
		f.End = ft.pc(currentIdx + 1)
		f.Name = funcName
		if found == len(funcs) {
			break
		}
	}
	return funcs
}

// funcName returns the name of the function found at off.
func (t *lineTable) funcName(off uint32) string {
	if n, err := t.funcnametab.ReadAt(t.funcNameHelper[23:], int64(off)+23); err != nil || n != len(t.funcNameHelper[23:]) {
		return ""
	}

	if t.funcNameHelper[23] != 0 && t.funcNameHelper[24] != 0 {
		return ""
	}

	if n, err := t.funcnametab.ReadAt(t.funcNameHelper[:23], int64(off)); err != nil || n != len(t.funcNameHelper[:23]) {
		return ""
	}

	if t.funcNameHelper[23] == 0 && isRelevantFunction(string(t.funcNameHelper[:23])) {
		return string(t.funcNameHelper[:23])
	}
	if t.funcNameHelper[24] == 0 && isRelevantFunction(string(t.funcNameHelper[:24])) {
		return string(t.funcNameHelper[:24])
	}

	return ""
}

func (t *lineTable) functabFieldSize() int {
	if t.Version >= ver118 {
		return 4
	}
	return int(t.Ptrsize)
}

func (t *lineTable) funcTab(cacheSize int) funcTab {
	a := t.functabFieldSize()
	return funcTab{lineTable: t, sz: a, funcTabHelper: make([]byte, 2*a*cacheSize)}
}

type funcTab struct {
	*lineTable
	sz            int // cached result of t.functabFieldSize
	funcTabHelper []byte
}

func (f funcTab) Count() int {
	return int(f.nfunctab)
}

func (f funcTab) pc(i int) uint64 {
	if n, err := f.functab.ReadAt(f.ptrBufferSizeHelper, int64(2*i*f.sz)); err != nil || n != int(f.Ptrsize) {
		return 0
	}
	u := f.uint(f.ptrBufferSizeHelper)
	if f.Version >= ver118 {
		u += f.textStart
	}
	return u
}

func (f funcTab) loadCache(start, count int) {
	if count == 0 {
		return
	}
	if n, err := f.functab.ReadAt(f.funcTabHelper, int64((2*start+1)*f.sz)); err != nil || n != len(f.funcTabHelper) {
		return
	}
}

func (f funcTab) funcOffFromCache(i int) uint64 {
	return f.uint(f.funcTabHelper[2*i*f.sz:])
}

func (f funcTab) uint(b []byte) uint64 {
	if f.sz == 4 {
		return uint64(f.Binary.Uint32(b))
	}
	return f.Binary.Uint64(b)
}

func field(ptrSize uint32, version version, binary binary.ByteOrder, data sectionAccess, helper []byte) uint32 {
	sz0 := ptrSize
	if version >= ver118 {
		sz0 = 4
	}
	off := sz0
	if n, err := data.ReadAt(helper, int64(off)); err != nil || n != int(ptrSize) {
		return 0
	}
	return binary.Uint32(helper)
}
