// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

//nolint:all
//go:noinline
func testSingleString(x string) {}

//nolint:all
//go:noinline
func testThreeStrings(x, y, z string) {}

type threeStringStruct struct {
	a string
	b string
	c string
}

type oneStringStruct struct {
	a string
}

//nolint:all
//go:noinline
func testThreeStringsInStruct(a threeStringStruct) {}

//nolint:all
//go:noinline
func testThreeStringsInStructPointer(a *threeStringStruct) {}

//nolint:all
//go:noinline
func testOneStringInStructPointer(a *oneStringStruct) {}

//nolint:all
//go:noinline
func testMassiveString(x string) {}

//nolint:all
//go:noinline
func testUnitializedString(x string) {}

//nolint:all
//go:noinline
func testEmptyString(x string) {}

//nolint:all
//go:noinline
func testSubstrings(a string, b string, c string) {}

//nolint:all
func executeStringFuncs() {
	abc := "abc"

	// Diagnostic: report residency of the .rodata pages holding the test
	// string literals BEFORE any user-side code reads them. Helps explain
	// bpf_probe_read_user -EFAULT failures on never-faulted pages.
	def := "def"
	ghi := "ghi"
	logVMAResidency("abc-vma", unsafe.StringData(abc))
	logRodataResidency("abc", unsafe.StringData(abc))
	logRodataResidency("def", unsafe.StringData(def))
	logRodataResidency("ghi", unsafe.StringData(ghi))

	testSingleString(abc)
	testThreeStrings(abc, "def", "ghi")
	testThreeStringsInStruct(threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testThreeStringsInStructPointer(&threeStringStruct{a: "abc", b: "def", c: "ghi"})
	testOneStringInStructPointer(&oneStringStruct{a: "omg"})
	testMassiveString(x)

	var uninitializedString string
	testUnitializedString(uninitializedString)
	testEmptyString("")
	testEmptyString(abc[:0])

	// Check captures when multiple variables are aliasing the same underlying buffer.
	s := "abcdef"
	testSubstrings(s[:4], s[:2], s)
}

var x = strings.Repeat("x", 100000)

// logVMAResidency finds the VMA containing p in /proc/self/maps and reports
// how many of its pages are resident (per /proc/self/pagemap bit 63).
//
//go:noinline
func logVMAResidency(label string, p *byte) {
	addr := uint64(uintptr(unsafe.Pointer(p)))
	mapsData, err := os.ReadFile("/proc/self/maps")
	if err != nil {
		fmt.Fprintf(os.Stderr, "vma[%s]: maps read: %v\n", label, err)
		return
	}
	var start, end uint64
	var line string
	for _, l := range strings.Split(string(mapsData), "\n") {
		dash := strings.IndexByte(l, '-')
		if dash < 0 {
			continue
		}
		sp := strings.IndexByte(l, ' ')
		if sp < 0 || sp <= dash {
			continue
		}
		s, err1 := strconv.ParseUint(l[:dash], 16, 64)
		e, err2 := strconv.ParseUint(l[dash+1:sp], 16, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if addr >= s && addr < e {
			start, end, line = s, e, l
			break
		}
	}
	if start == 0 {
		fmt.Fprintf(os.Stderr, "vma[%s]: VMA not found for %#x\n", label, addr)
		return
	}
	pageSize := uint64(os.Getpagesize())
	nPages := (end - start) / pageSize
	pm, err := os.Open("/proc/self/pagemap")
	if err != nil {
		fmt.Fprintf(os.Stderr, "vma[%s]: pagemap open: %v\n", label, err)
		return
	}
	defer pm.Close()
	buf := make([]byte, nPages*8)
	if _, err := pm.ReadAt(buf, int64(start/pageSize)*8); err != nil {
		fmt.Fprintf(os.Stderr, "vma[%s]: pagemap read: %v\n", label, err)
		return
	}
	var present, swapped uint64
	for i := uint64(0); i < nPages; i++ {
		e := binary.LittleEndian.Uint64(buf[i*8 : (i+1)*8])
		if e>>63&1 == 1 {
			present++
		}
		if e>>62&1 == 1 {
			swapped++
		}
	}
	fmt.Fprintf(os.Stderr, "vma[%s]: %s\n", label, line)
	fmt.Fprintf(os.Stderr,
		"vma[%s]: %d/%d pages resident (%d/%d KiB), %d swapped\n",
		label, present, nPages, present*pageSize/1024, nPages*pageSize/1024, swapped,
	)
}

// logRodataResidency reports whether the page containing p is resident.
// It reads /proc/self/pagemap (bit 63 of the per-page entry indicates the
// page is present in RAM) without touching the bytes at p.
//
//go:noinline
func logRodataResidency(label string, p *byte) {
	addr := uintptr(unsafe.Pointer(p))
	pageSize := uintptr(os.Getpagesize())
	pageStart := addr &^ (pageSize - 1)
	pageIdx := pageStart / pageSize

	pm, err := os.Open("/proc/self/pagemap")
	if err != nil {
		fmt.Fprintf(os.Stderr, "residency[%s]: pagemap open: %v\n", label, err)
		return
	}
	defer pm.Close()
	var buf [8]byte
	if _, err := pm.ReadAt(buf[:], int64(pageIdx)*8); err != nil {
		fmt.Fprintf(os.Stderr, "residency[%s]: pagemap read: %v\n", label, err)
		return
	}
	entry := binary.LittleEndian.Uint64(buf[:])
	present := entry>>63&1 == 1
	swapped := entry>>62&1 == 1
	fmt.Fprintf(os.Stderr,
		"residency[%s]: addr=%#x page=%#x present=%v swapped=%v entry=%#016x\n",
		label, addr, pageStart, present, swapped, entry,
	)
}
