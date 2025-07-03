// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loclist supports processing DWARF loclists.
package loclist

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// fixLoclists mitigates bug in go that can cause incorrect location lists to be generated.
// The bug occurs because of some behavior in ssa where different pieces of a
// variable can be interpreted as different types and can end up having
// duplicated location lists. Empirically this has primarily been observed with
// contexts and when it occurs, the location lists are doubled. Below is a
// somewhat crude heuristic to detect this case and fix it.
//
// See https://github.com/golang/go/issues/61700
func fixLoclists(loclists []ir.Location, expectedByteSize uint32) ([]ir.Location, error) {
	currentByteSize := func(loclists []ir.Location) uint32 {
		if len(loclists) == 0 || len(loclists[0].Pieces) == 0 {
			return 0
		}
		var sum uint32
		for _, piece := range loclists[0].Pieces {
			sum += piece.Size
		}
		return sum
	}

	// We want to find any pair of adjacent pieces that are in the same location
	for currentByteSize(loclists) > expectedByteSize {
		if len(loclists) == 0 || len(loclists[0].Pieces) == 0 {
			break
		}

		numPieces := len(loclists[0].Pieces)
		var idx int
		var found bool

		// First strategy: find adjacent pieces with same byte size and same operation
		for _, loclist := range loclists {
			for i := 0; i < len(loclist.Pieces)-1; i++ {
				a, b := &loclist.Pieces[i], &loclist.Pieces[i+1]
				if a.Size == b.Size &&
					a.Op != nil && b.Op != nil &&
					a.Op == b.Op {
					idx = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		// Second strategy: find pieces where there are no conflicts in their location
		// because it's always either one is present or the other but not both
		if !found {
			for i := 1; i < numPieces; i++ {
				allMatch := true
				for _, loclist := range loclists {
					if i >= len(loclist.Pieces) {
						allMatch = false
						break
					}
					a, b := &loclist.Pieces[i-1], &loclist.Pieces[i]
					matched := a.Size == b.Size && (a.Op == nil) != (b.Op == nil)
					if !matched {
						allMatch = false
						break
					}
				}
				if allMatch {
					idx = i - 1
					found = true
					break
				}
			}
		}

		// Third strategy: find adjacent pieces with same byte size where both ops are nil
		// and the next piece has a non-nil op
		if !found {
			for i := 1; i < numPieces; i++ {
				allMatch := true
				for _, loclist := range loclists {
					if i >= len(loclist.Pieces) {
						allMatch = false
						break
					}
					a, b := &loclist.Pieces[i-1], &loclist.Pieces[i]
					matched := a.Size == b.Size && a.Op == nil && b.Op == nil
					if !matched {
						allMatch = false
						break
					}
					// Check if next piece has non-nil op (or we're at the end)
					if i+1 < len(loclist.Pieces) {
						next := &loclist.Pieces[i+1]
						if next.Op == nil {
							allMatch = false
							break
						}
					}
				}
				if allMatch {
					idx = i - 1
					found = true
					break
				}
			}
		}

		if !found {
			return nil, fmt.Errorf("could not compact pieces")
		}

		// Remove and merge pieces at the found index
		for i := range loclists {
			if idx >= len(loclists[i].Pieces) || idx+1 >= len(loclists[i].Pieces) {
				continue
			}

			removed := loclists[i].Pieces[idx]
			// Remove the piece at foundIndex
			loclists[i].Pieces = append(loclists[i].Pieces[:idx], loclists[i].Pieces[idx+1:]...)

			// Merge with the piece that's now at foundIndex (was at foundIndex+1)
			if idx < len(loclists[i].Pieces) {
				other := &loclists[i].Pieces[idx]
				if removed.Op != nil {
					other.Op = removed.Op
				}
			}
		}
	}

	return loclists, nil
}
