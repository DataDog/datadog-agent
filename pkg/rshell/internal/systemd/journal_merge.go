// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const (
	maxJournalResultBytes = 8 * 1024 * 1024
)

type journalMergeSource struct {
	iterator *journalFileQueryIterator
	current  *journalQueryEntry
	done     bool
}

func (s *journalMergeSource) load(ctx context.Context) error {
	if s.current != nil || s.done {
		return nil
	}
	entry, found, err := s.iterator.previous(ctx)
	if err != nil {
		return err
	}
	if !found {
		s.done = true
		return nil
	}
	s.current = &entry
	return nil
}

func (c *Client) queryJournalEntries(ctx context.Context, query builtins.JournalQuery) ([]journalQueryEntry, error) {
	if err := validateJournalQuery(query); err != nil {
		return nil, err
	}
	if query.MaxEntries == 0 {
		return nil, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		entries, err := c.queryJournalEntriesOnce(ctx, query)
		if err == nil {
			return entries, nil
		}
		if attempt == 0 && (errors.Is(err, errJournalChanged) || errors.Is(err, errJournalCorrupt)) {
			continue
		}
		return nil, err
	}
	return nil, journalChanged("journal did not stabilize after retry")
}

func (c *Client) queryJournalEntriesOnce(ctx context.Context, query builtins.JournalQuery) ([]journalQueryEntry, error) {
	snapshot, err := c.openJournalSnapshot()
	if err != nil {
		return nil, err
	}
	entries, queryErr := queryJournalSnapshot(ctx, snapshot, query)
	if queryErr == nil {
		queryErr = snapshot.stable(c)
	}
	closeErr := snapshot.close()
	if queryErr != nil {
		return nil, queryErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return entries, nil
}

func queryJournalSnapshot(ctx context.Context, snapshot *journalSnapshot, query builtins.JournalQuery) ([]journalQueryEntry, error) {
	var currentBoot *journalID
	if query.CurrentBoot {
		bootID, err := newestJournalBoot(snapshot)
		if err != nil {
			return nil, err
		}
		currentBoot = &bootID
	}

	sources := make([]*journalMergeSource, 0, len(snapshot.files))
	for _, opened := range snapshot.files {
		iterator, err := newJournalFileQueryIterator(opened.view, query, currentBoot)
		if err != nil {
			return nil, err
		}
		sources = append(sources, &journalMergeSource{iterator: iterator})
	}

	entries := make([]journalQueryEntry, 0, query.MaxEntries)
	resultBytes := 0
	for len(entries) < query.MaxEntries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var newest *journalQueryEntry
		for _, source := range sources {
			if err := source.load(ctx); err != nil {
				return nil, err
			}
			if source.current != nil && (newest == nil || compareJournalQueryEntries(*source.current, *newest) > 0) {
				newest = source.current
			}
		}
		if newest == nil {
			break
		}

		selected := *newest
		for _, source := range sources {
			if source.current == nil || !sameJournalEntryLocation(*source.current, selected) {
				continue
			}
			if source.current.selected != selected.selected {
				return nil, journalCorrupt(source.iterator.file.name, source.current.offset, "duplicate journal location has conflicting selected fields")
			}
			source.current = nil
		}

		resultBytes += len(selected.selected.Hostname) + len(selected.selected.Identifier) + len(selected.selected.PID) + len(selected.selected.Message)
		if resultBytes > maxJournalResultBytes {
			return nil, fmt.Errorf("%w: selected journal fields exceed %d bytes", errJournalLimit, maxJournalResultBytes)
		}
		entries = append(entries, selected)
	}
	return entries, nil
}

func newestJournalBoot(snapshot *journalSnapshot) (journalID, error) {
	var newest *journalQueryEntry
	for _, opened := range snapshot.files {
		entry, found, err := newestJournalEntry(opened.view)
		if err != nil {
			return journalID{}, err
		}
		if found && (newest == nil || compareJournalQueryEntries(entry, *newest) > 0) {
			copy := entry
			newest = &copy
		}
	}
	if newest == nil {
		return journalID{}, errors.New("could not determine the current boot from the selected journal")
	}
	return newest.bootID, nil
}

func newestJournalEntry(file *journalFileView) (journalQueryEntry, bool, error) {
	if file.header.nEntries == 0 {
		return journalQueryEntry{}, false, nil
	}

	var offset uint64
	if file.header.hasTailEntryOffset {
		if file.header.tailEntryOffset == 0 {
			return journalQueryEntry{}, false, journalCorrupt(file.name, 264, "journal has entries but no tail ENTRY offset")
		}
		offset = file.header.tailEntryOffset
	} else {
		iterator, err := file.globalEntryOffsets()
		if err != nil {
			return journalQueryEntry{}, false, err
		}
		var found bool
		offset, found, err = iterator.previous()
		if err != nil {
			return journalQueryEntry{}, false, err
		}
		if !found {
			return journalQueryEntry{}, false, journalCorrupt(file.name, file.header.entryArrayOffset, "global ENTRY_ARRAY is empty")
		}
	}

	entry, err := file.entryObjectAt(offset)
	if err != nil {
		return journalQueryEntry{}, false, err
	}
	if entry.seqnum == 0 || entry.realtime == 0 || entry.bootID.zero() {
		return journalQueryEntry{}, false, journalCorrupt(file.name, entry.offset, "tail ENTRY has invalid sequence, timestamp, or boot metadata")
	}
	if file.header.tailEntrySeqnum != 0 && file.header.tailEntrySeqnum != entry.seqnum {
		return journalQueryEntry{}, false, journalCorrupt(file.name, entry.offset, "tail ENTRY sequence %d does not match header sequence %d", entry.seqnum, file.header.tailEntrySeqnum)
	}
	return journalQueryEntry{
		fileID:    file.header.fileID,
		seqnumID:  file.header.seqnumID,
		bootID:    entry.bootID,
		seqnum:    entry.seqnum,
		realtime:  entry.realtime,
		monotonic: entry.monotonic,
		xorHash:   entry.xorHash,
		offset:    entry.offset,
	}, true, nil
}

func (f *journalFileView) globalEntryOffsets() (*journalEntryOffsetIterator, error) {
	iterator := &journalEntryOffsetIterator{file: f, segmentIndex: -1}
	if f.header.nEntries == 0 {
		return iterator, nil
	}
	if f.header.entryArrayOffset == 0 {
		return nil, journalCorrupt(f.name, 176, "journal has entries but no global ENTRY_ARRAY")
	}

	remaining := f.header.nEntries
	offset := f.header.entryArrayOffset
	seen := make(map[uint64]struct{}, maxJournalEntryArrays)
	for remaining > 0 {
		if len(iterator.segments) >= maxJournalEntryArrays {
			return nil, journalLimit(f.name, offset, "global ENTRY_ARRAY chain exceeds %d objects", maxJournalEntryArrays)
		}
		if _, exists := seen[offset]; exists {
			return nil, journalCorrupt(f.name, offset, "global ENTRY_ARRAY chain contains a cycle")
		}
		seen[offset] = struct{}{}

		array, err := f.entryArrayAt(offset)
		if err != nil {
			return nil, err
		}
		count := array.capacity
		if count > remaining {
			count = remaining
		}
		iterator.segments = append(iterator.segments, journalEntryArraySegment{array: array, count: count})
		remaining -= count
		if remaining == 0 {
			if array.nextOffset != 0 {
				return nil, journalCorrupt(f.name, array.offset, "global ENTRY_ARRAY continues after all entries")
			}
			break
		}
		if array.nextOffset == 0 {
			return nil, journalCorrupt(f.name, array.offset, "global ENTRY_ARRAY ends before all entries")
		}
		offset = array.nextOffset
	}

	if f.header.hasTailEntryArray {
		last := iterator.segments[len(iterator.segments)-1]
		if uint64(f.header.tailEntryArrayOffset) != last.array.offset || uint64(f.header.tailEntryArrayNEntries) != last.count {
			return nil, journalCorrupt(f.name, 256, "global tail ENTRY_ARRAY metadata does not match its array chain")
		}
	}
	iterator.segmentIndex = len(iterator.segments) - 1
	iterator.itemIndex = iterator.segments[iterator.segmentIndex].count
	return iterator, nil
}

func compareJournalQueryEntries(left, right journalQueryEntry) int {
	if !left.seqnumID.zero() && left.seqnumID == right.seqnumID && left.seqnum != right.seqnum {
		return compareUint64(left.seqnum, right.seqnum)
	}
	if !left.bootID.zero() && left.bootID == right.bootID && left.monotonic != right.monotonic {
		return compareUint64(left.monotonic, right.monotonic)
	}
	if left.realtime != right.realtime {
		return compareUint64(left.realtime, right.realtime)
	}
	if left.xorHash != right.xorHash {
		return compareUint64(left.xorHash, right.xorHash)
	}
	if compared := bytes.Compare(left.fileID[:], right.fileID[:]); compared != 0 {
		return compared
	}
	return compareUint64(left.offset, right.offset)
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func sameJournalEntryLocation(left, right journalQueryEntry) bool {
	return !left.seqnumID.zero() &&
		left.seqnumID == right.seqnumID &&
		left.seqnum == right.seqnum &&
		left.bootID == right.bootID &&
		left.realtime == right.realtime &&
		left.monotonic == right.monotonic &&
		left.xorHash == right.xorHash
}
