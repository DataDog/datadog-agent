// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const (
	maxJournalFuzzInput       = 256 * 1024
	maxJournalFuzzEncodedData = 64 * 1024
	maxJournalFuzzPayloadRead = 4 * 1024
	journalFuzzTimeout        = 10 * time.Millisecond
)

func FuzzJournalLZ4Prefix(f *testing.F) {
	f.Add(append([]byte{0x90}, []byte("MESSAGE=x")...), uint64(9), uint16(4))
	f.Add([]byte{0x1f, 'a', 0x01, 0x00, 12}, uint64(32), uint16(8))

	f.Fuzz(func(_ *testing.T, compressed []byte, rawDecodedSize uint64, rawLimit uint16) {
		if len(compressed) == 0 || len(compressed) > maxJournalFuzzEncodedData {
			return
		}
		decodedSize := rawDecodedSize%maxJournalLZ4DataSize + 1
		limit := int(rawLimit%maxJournalFuzzPayloadRead) + 1
		_, _, _ = decodeJournalLZ4Prefix(bytes.NewReader(compressed), uint64(len(compressed)), decodedSize, limit)
	})
}

// FuzzJournalObjects drives every bounded object decoder reachable from a
// parsed header. Corrupt and unsupported inputs are expected; the contract is
// that they return without panicking or allocating from untrusted sizes.
func FuzzJournalObjects(f *testing.F) {
	f.Add([]byte{}, []byte("MESSAGE=x"), uint64(0))
	f.Add(testJournalContents(journalHeaderCurrentSize, 0, 0), []byte("MESSAGE=x"), uint64(journalHeaderCurrentSize))
	f.Add(buildJournalFuzzSeed(f, false), []byte("_SYSTEMD_UNIT=api.service"), uint64(journalHeaderCurrentSize))
	f.Add(buildJournalFuzzSeed(f, true), []byte("MESSAGE=ready"), uint64(journalHeaderCurrentSize))

	f.Fuzz(func(_ *testing.T, contents, selector []byte, rawOffset uint64) {
		if len(contents) > maxJournalFuzzInput || len(selector) == 0 || len(selector) > 512 {
			return
		}
		view, err := newJournalFileView("fuzz.journal", bytes.NewReader(contents), uint64(len(contents)))
		if err != nil {
			return
		}

		offsets := []uint64{
			rawOffset &^ 7,
			view.header.tailObjectOffset,
			view.header.tailEntryOffset,
		}
		if view.header.dataHashTableOffset >= journalObjectHeaderSize {
			offsets = append(offsets, view.header.dataHashTableOffset-journalObjectHeaderSize)
		}
		for _, offset := range offsets {
			fuzzJournalObject(view, offset)
		}

		data, found, err := view.findDataObject(selector)
		if err == nil && found && data.payloadSize <= maxJournalFuzzEncodedData {
			_, _, _ = view.readDataPayload(data, maxJournalFuzzPayloadRead)
		}
		_, _, _ = newestJournalEntry(view)
	})
}

// FuzzJournalQuery mutates structurally valid regular and compact journals and
// runs the same indexed unit/kernel query path used by the builtin.
func FuzzJournalQuery(f *testing.F) {
	f.Add(buildJournalFuzzSeed(f, false), "api.service", false, byte(10))
	f.Add(buildJournalFuzzSeed(f, true), "api.service", false, byte(10))
	f.Add(buildJournalFuzzSeed(f, false), "", true, byte(10))

	f.Fuzz(func(t *testing.T, contents []byte, unit string, kernel bool, rawLimit byte) {
		if len(contents) > maxJournalFuzzInput || len(unit) > 256 {
			return
		}
		view, err := newJournalFileView("query-fuzz.journal", bytes.NewReader(contents), uint64(len(contents)))
		if err != nil {
			return
		}
		if view.header.incompatibleFlags&(journalHeaderIncompatibleCompressedXZ|journalHeaderIncompatibleCompressedLZ4|journalHeaderIncompatibleCompressedZSTD) != 0 {
			return
		}

		query := builtins.JournalQuery{Kernel: kernel, MaxEntries: int(rawLimit%10) + 1}
		if !kernel {
			query.Units = []string{unit}
		}
		if err := validateJournalQuery(query); err != nil {
			return
		}

		iterator, err := newJournalFileQueryIterator(view, query, nil)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), journalFuzzTimeout)
		defer cancel()
		for count := 0; count < query.MaxEntries; count++ {
			entry, found, err := iterator.previous(ctx)
			if err != nil || !found {
				return
			}
			if len(entry.selected.Hostname) > maxJournalFieldSize ||
				len(entry.selected.Identifier) > maxJournalFieldSize ||
				len(entry.selected.PID) > maxJournalFieldSize ||
				len(entry.selected.Message) > maxJournalFieldSize {
				t.Fatal("journal query returned a selected field over its size limit")
			}
		}
	})
}

func fuzzJournalObject(view *journalFileView, offset uint64) {
	if offset == 0 {
		return
	}
	object, err := view.objectAt(offset, 0)
	if err != nil {
		return
	}
	switch object.objectType {
	case journalObjectData:
		data, err := view.dataObjectAt(offset)
		if err == nil && data.payloadSize <= maxJournalFuzzEncodedData {
			_, _, _ = view.readDataPayload(data, maxJournalFuzzPayloadRead)
		}
	case journalObjectEntry:
		_, _ = view.entryObjectAt(offset)
	case journalObjectEntryArray:
		array, err := view.entryArrayAt(offset)
		if err == nil && array.capacity > 0 {
			_, _ = view.entryArrayItem(array, 0)
		}
	}
}

func buildJournalFuzzSeed(t testing.TB, compact bool) []byte {
	t.Helper()
	bootID := repeatedJournalID(0x22)
	return buildJournalQueryFixtureWithLayout(t, []journalQueryFixtureEntry{
		{bootID: bootID, realtime: 1_700_000_000_000_000, fields: []string{"_SYSTEMD_UNIT=api.service", "_HOSTNAME=node", "SYSLOG_IDENTIFIER=api", "_PID=42", "MESSAGE=ready"}},
		{bootID: bootID, realtime: 1_700_000_000_000_001, fields: []string{"UNIT=api.service", "_PID=1", "SYSLOG_IDENTIFIER=systemd", "MESSAGE=manager"}},
		{bootID: bootID, realtime: 1_700_000_000_000_002, fields: []string{"_TRANSPORT=kernel", "_COMM=kernel", "MESSAGE=kernel"}},
	}, compact)
}
