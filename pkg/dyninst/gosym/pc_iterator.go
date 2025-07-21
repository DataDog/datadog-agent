// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym

import "fmt"

// PCIterator iterates over program counter ranges within a single function,
// mapping them to source line numbers.
type PCIterator struct {
	stepper stepper

	// inlinedPcRanges is a list of program counter ranges that correspond to
	// other functions inlined within the current function. The ranges are
	// inclusive on the left, exclusive on the right, disjoint and ordered.
	inlinedPcRanges [][2]uint64
	// A cursor into inlinedPcRanges positioned corresponding to the stepper.pc:
	// inlinedPcRanges[inlinedPcRangesIdx][1] <= stepper.pcStart < inlinedPcRanges[inlinedPcRangesIdx][1]
	inlinedPcRangesIdx int
}

// makeFuncPCIterator builds a PCIterator for the function described by fi.
func makeFuncPCIterator(fi *funcInfo, inlinedPcRanges [][2]uint64) (PCIterator, error) {
	pcLine, found := fi.pcln()
	if !found {
		return PCIterator{}, fmt.Errorf("failed to find line table entry for function")
	}

	offset := fi.lt.pcTab[0] + int(pcLine)
	if offset >= len(fi.lt.data) {
		return PCIterator{}, fmt.Errorf("function line table offset out of bounds")
	}
	cursor := fi.lt.data[offset:]
	entryPC, ok := fi.entryPC()
	if !ok {
		return PCIterator{}, fmt.Errorf("failed to get function entry PC")
	}
	stepper := makeStepper(cursor, entryPC, fi.lt.quantum)

	return PCIterator{
		stepper:            stepper,
		inlinedPcRanges:    inlinedPcRanges,
		inlinedPcRangesIdx: 0,
	}, nil
}

// Reset resets the iterator to the beginning of the function. After Reset,
// Next() will return the first PC range of the function.
func (f *PCIterator) Reset() {
	f.stepper.reset()
	f.inlinedPcRangesIdx = 0
}

// PCToLine returns the line number corresponding to the given program counter.
// Only line numbers that correspond to the iterator's own function are
// returned; if the PC corresponds to an inlined function, returns (0, false).
//
// Upon return, the iterator is positioned on the PC range containing the given
// PC.
//
// Any PC inside the function is a valid input, but the PCIterator is more
// efficient if PCToLine is called with monotonically increasing PCs.
//
// Returns false if the PC could not be resolved to a line number.
func (f *PCIterator) PCToLine(pc uint64) (uint32, bool) {
	if f.stepper.pcEnd > pc {
		// We cannot step backwards, so re-initialize the iterator and
		// start the iteration from the beginning of the function.
		f.Reset()
	}

	// Advance the iterator to the right position.
	var mapping PCRangeMapping
	for f.stepper.first || pc >= f.stepper.pcEnd {
		var ok bool
		mapping, ok = f.Next()
		if !ok {
			return 0, false
		}
	}
	return mapping.Line, true
}

// PCRangeMapping represents a mapping from a range of program counters to a
// line of source code.
type PCRangeMapping struct {
	// The start of the range; inclusive.
	PCStart uint64
	// The end of the range; exclusive.
	PCEnd uint64
	// The line that the range [PCStart, PCEnd) maps to.
	Line uint32
}

// Next advances the iterator to the next PC range, skipping over ranges
// corresponding to inlined code. Returns information about the current PC
// range. Returns PCRangeMapping{}, false if the iterator has been exhausted.
//
// Calling Next() again after it has returned false is a no-op that will
// continue returning false.
func (f *PCIterator) Next() (PCRangeMapping, bool) {
	// Advance the stepper possibly multiple times to skip over ranges
	// corresponding to inlined functions.
	for {
		if !f.stepper.step() {
			return PCRangeMapping{}, false
		}
		f.advanceInlinedRanges()
		if !f.pcInInlinedFunc() {
			break
		}
	}

	return PCRangeMapping{
		PCStart: f.stepper.pcStart,
		PCEnd:   f.stepper.pcEnd,
		Line:    uint32(f.stepper.val),
	}, true
}

// advanceInlinedRanges moves up f.inlinedPcRangesIdx such that it points after all the ranges
// that end before the current PC range (i.e. before f.stepper.pcStart).
func (f *PCIterator) advanceInlinedRanges() {
	for f.inlinedPcRangesIdx < len(f.inlinedPcRanges) {
		if f.stepper.pcStart >= f.inlinedPcRanges[f.inlinedPcRangesIdx][1] {
			f.inlinedPcRangesIdx++
		} else {
			break
		}
	}
}

// pcInInlinedFunc checks if the current PC range overlaps with the current
// inlined function range.
func (f *PCIterator) pcInInlinedFunc() bool {
	if f.inlinedPcRangesIdx >= len(f.inlinedPcRanges) {
		return false
	}

	inlinedRange := f.inlinedPcRanges[f.inlinedPcRangesIdx]
	// inlinedPcRangesIdx is supposed to be positioned after all the inlined
	// ranges below the current PC range.
	if inlinedRange[1] <= f.stepper.pcStart {
		panic("inlinedPcRangesIdx out of sync with stepper.pcStart")
	}

	return f.stepper.pcStart >= f.inlinedPcRanges[f.inlinedPcRangesIdx][0]
}
