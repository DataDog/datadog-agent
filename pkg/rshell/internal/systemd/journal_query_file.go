// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package systemd

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const (
	maxJournalCandidatesScanned = 100_000
	maxJournalFieldNameSize     = 64
	journalCoredumpMessageID    = "fc2e22bc6ee647b6b90729ab34a250b1"
)

type journalQueryEntry struct {
	fileID    journalID
	seqnumID  journalID
	bootID    journalID
	seqnum    uint64
	realtime  uint64
	monotonic uint64
	xorHash   uint64
	offset    uint64
	selected  builtins.JournalEntry
}

type journalDataCandidateSource struct {
	iterator *journalEntryOffsetIterator
	index    journalDataObject
	required []journalDataObject
	current  *journalEntryObject
	done     bool
}

func (s *journalDataCandidateSource) load(ctx context.Context, scanned *int) error {
	if s.current != nil || s.done {
		return nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		offset, found, err := s.iterator.previous()
		if err != nil {
			return err
		}
		if !found {
			s.done = true
			return nil
		}
		if *scanned >= maxJournalCandidatesScanned {
			return journalLimit(s.iterator.file.name, offset, "query examines more than %d indexed candidates", maxJournalCandidatesScanned)
		}
		(*scanned)++

		entry, err := s.iterator.file.entryObjectAt(offset)
		if err != nil {
			return err
		}
		indexed, err := journalEntryContainsData(s.iterator.file, entry, s.index)
		if err != nil {
			return err
		}
		if !indexed {
			return journalCorrupt(s.iterator.file.name, offset, "DATA entry index points to an ENTRY that does not reference the DATA object")
		}
		matches := true
		for _, required := range s.required {
			contains, err := journalEntryContainsData(s.iterator.file, entry, required)
			if err != nil {
				return err
			}
			if !contains {
				matches = false
				break
			}
		}
		if matches {
			s.current = &entry
			return nil
		}
	}
}

func journalEntryContainsData(file *journalFileView, entry journalEntryObject, data journalDataObject) (bool, error) {
	for index, item := range entry.items {
		if item.dataOffset != data.offset {
			continue
		}
		if !file.header.compact() && item.hash != data.hash {
			return false, journalCorrupt(file.name, entry.offset, "ENTRY item %d hash does not match its DATA object", index)
		}
		return true, nil
	}
	return false, nil
}

type journalFileQueryIterator struct {
	file       *journalFileView
	sources    []*journalDataCandidateSource
	bootID     journalID
	filterBoot bool
	sinceUsec  uint64
	scanned    int
}

func newJournalFileQueryIterator(file *journalFileView, query builtins.JournalQuery, currentBoot *journalID) (*journalFileQueryIterator, error) {
	if err := validateJournalQuery(query); err != nil {
		return nil, err
	}
	iterator := &journalFileQueryIterator{file: file}
	if query.CurrentBoot {
		if currentBoot == nil {
			return nil, fmt.Errorf("current journal boot is not available")
		}
		iterator.bootID = *currentBoot
		iterator.filterBoot = true
	}
	if !query.Since.IsZero() && query.Since.UnixMicro() > 0 {
		iterator.sinceUsec = uint64(query.Since.UnixMicro())
	}

	if query.Kernel {
		data, found, err := file.findDataObject([]byte("_TRANSPORT=kernel"))
		if err != nil {
			return nil, err
		}
		if found {
			if err := iterator.addSource(data); err != nil {
				return nil, err
			}
		}
		return iterator, nil
	}

	// Older systemd releases match PID 1; current releases match init.scope so
	// helper processes forked by the manager are included too.
	pidOne, pidOneFound, err := file.findDataObject([]byte("_PID=1"))
	if err != nil {
		return nil, err
	}
	initScope, initScopeFound, err := file.findDataObject([]byte("_SYSTEMD_CGROUP=/init.scope"))
	if err != nil {
		return nil, err
	}
	rootUID, rootUIDFound, err := file.findDataObject([]byte("_UID=0"))
	if err != nil {
		return nil, err
	}
	coredumpMessage, coredumpMessageFound, err := file.findDataObject([]byte("MESSAGE_ID=" + journalCoredumpMessageID))
	if err != nil {
		return nil, err
	}
	seenUnits := make(map[string]struct{}, len(query.Units))
	for _, unit := range query.Units {
		if _, exists := seenUnits[unit]; exists {
			continue
		}
		seenUnits[unit] = struct{}{}
		direct, found, err := file.findDataObject([]byte("_SYSTEMD_UNIT=" + unit))
		if err != nil {
			return nil, err
		}
		if found {
			if err := iterator.addSource(direct); err != nil {
				return nil, err
			}
		}

		manager, found, err := file.findDataObject([]byte("UNIT=" + unit))
		if err != nil {
			return nil, err
		}
		if found && pidOneFound {
			if err := iterator.addSource(manager, pidOne); err != nil {
				return nil, err
			}
		}
		if found && initScopeFound {
			if err := iterator.addSource(manager, initScope); err != nil {
				return nil, err
			}
		}

		object, found, err := file.findDataObject([]byte("OBJECT_SYSTEMD_UNIT=" + unit))
		if err != nil {
			return nil, err
		}
		if found && rootUIDFound {
			if err := iterator.addSource(object, rootUID); err != nil {
				return nil, err
			}
		}

		coredump, found, err := file.findDataObject([]byte("COREDUMP_UNIT=" + unit))
		if err != nil {
			return nil, err
		}
		if found && rootUIDFound && coredumpMessageFound {
			if err := iterator.addSource(coredump, rootUID, coredumpMessage); err != nil {
				return nil, err
			}
		}

		if strings.HasSuffix(unit, ".slice") {
			slice, found, err := file.findDataObject([]byte("_SYSTEMD_SLICE=" + unit))
			if err != nil {
				return nil, err
			}
			if found {
				if err := iterator.addSource(slice); err != nil {
					return nil, err
				}
			}
		}
	}
	return iterator, nil
}

func (i *journalFileQueryIterator) addSource(index journalDataObject, required ...journalDataObject) error {
	offsets, err := i.file.entryOffsetsForData(index)
	if err != nil {
		return err
	}
	i.sources = append(i.sources, &journalDataCandidateSource{iterator: offsets, index: index, required: required})
	return nil
}

func (i *journalFileQueryIterator) previous(ctx context.Context) (journalQueryEntry, bool, error) {
	for {
		entry, found, err := i.previousCandidate(ctx)
		if err != nil || !found {
			return journalQueryEntry{}, found, err
		}
		// Boot IDs form append-order segments, so earlier candidates cannot
		// re-enter the current boot once reverse traversal leaves it.
		if i.filterBoot && entry.bootID != i.bootID {
			return journalQueryEntry{}, false, nil
		}
		if i.sinceUsec != 0 && entry.realtime < i.sinceUsec {
			continue
		}
		if entry.seqnum == 0 {
			return journalQueryEntry{}, false, journalCorrupt(i.file.name, entry.offset, "ENTRY sequence number is zero")
		}
		if entry.bootID.zero() {
			return journalQueryEntry{}, false, journalCorrupt(i.file.name, entry.offset, "ENTRY boot ID is zero")
		}
		if entry.realtime == 0 || entry.realtime > math.MaxInt64 {
			return journalQueryEntry{}, false, journalCorrupt(i.file.name, entry.offset, "ENTRY realtime timestamp %d is invalid", entry.realtime)
		}

		selected, err := i.file.materializeJournalEntry(ctx, entry)
		if err != nil {
			return journalQueryEntry{}, false, err
		}
		return journalQueryEntry{
			fileID:    i.file.header.fileID,
			seqnumID:  i.file.header.seqnumID,
			bootID:    entry.bootID,
			seqnum:    entry.seqnum,
			realtime:  entry.realtime,
			monotonic: entry.monotonic,
			xorHash:   entry.xorHash,
			offset:    entry.offset,
			selected:  selected,
		}, true, nil
	}
}

func (i *journalFileQueryIterator) previousCandidate(ctx context.Context) (journalEntryObject, bool, error) {
	var newest *journalEntryObject
	for _, source := range i.sources {
		if err := source.load(ctx, &i.scanned); err != nil {
			return journalEntryObject{}, false, err
		}
		if source.current != nil && (newest == nil || source.current.offset > newest.offset) {
			newest = source.current
		}
	}
	if newest == nil {
		return journalEntryObject{}, false, nil
	}

	selected := *newest
	for _, source := range i.sources {
		if source.current != nil && source.current.offset == selected.offset {
			source.current = nil
		}
	}
	return selected, true, nil
}

func (f *journalFileView) materializeJournalEntry(ctx context.Context, entry journalEntryObject) (builtins.JournalEntry, error) {
	selected := builtins.JournalEntry{
		Timestamp: time.UnixMicro(int64(entry.realtime)),
	}
	var syslogIdentifier, command string
	var haveHostname, haveSyslogIdentifier, haveCommand, havePID, haveMessage bool
	seen := make(map[uint64]struct{}, len(entry.items))

	for index, item := range entry.items {
		if err := ctx.Err(); err != nil {
			return builtins.JournalEntry{}, err
		}
		if _, exists := seen[item.dataOffset]; exists {
			return builtins.JournalEntry{}, journalCorrupt(f.name, entry.offset, "ENTRY contains duplicate DATA offset %d", item.dataOffset)
		}
		seen[item.dataOffset] = struct{}{}

		data, err := f.dataObjectAt(item.dataOffset)
		if err != nil {
			return builtins.JournalEntry{}, err
		}
		if !f.header.compact() && item.hash != data.hash {
			return builtins.JournalEntry{}, journalCorrupt(f.name, entry.offset, "ENTRY item %d hash does not match its DATA object", index)
		}
		field, value, selectedField, err := f.selectedDataField(data)
		if err != nil {
			return builtins.JournalEntry{}, err
		}
		if !selectedField {
			continue
		}
		switch field {
		case "_HOSTNAME":
			if !haveHostname {
				selected.Hostname = value
				haveHostname = true
			}
		case "SYSLOG_IDENTIFIER":
			if !haveSyslogIdentifier {
				syslogIdentifier = value
				haveSyslogIdentifier = true
			}
		case "_COMM":
			if !haveCommand {
				command = value
				haveCommand = true
			}
		case "_PID":
			if !havePID {
				selected.PID = value
				havePID = true
			}
		case "MESSAGE":
			if !haveMessage {
				selected.Message = value
				haveMessage = true
			}
		}
	}
	selected.Identifier = syslogIdentifier
	if selected.Identifier == "" {
		selected.Identifier = command
	}
	return selected, nil
}

func (f *journalFileView) selectedDataField(data journalDataObject) (string, string, bool, error) {
	prefix, _, err := f.readDataPayload(data, maxJournalFieldNameSize+1)
	if err != nil {
		return "", "", false, err
	}
	separator := bytes.IndexByte(prefix, '=')
	if separator <= 0 || separator > maxJournalFieldNameSize {
		return "", "", false, journalCorrupt(f.name, data.payloadOffset, "DATA payload has an invalid field name")
	}
	fieldBytes := prefix[:separator]
	if !validJournalFieldName(fieldBytes) {
		return "", "", false, journalCorrupt(f.name, data.payloadOffset, "DATA payload has an invalid field name")
	}
	field := string(fieldBytes)
	if !selectedJournalField(field) {
		return field, "", false, nil
	}

	valueOffset := separator + 1
	limit := valueOffset + maxJournalFieldSize
	payload, truncated, err := f.readDataPayload(data, limit)
	if err != nil {
		return "", "", false, err
	}
	if !bytes.HasPrefix(payload, prefix[:valueOffset]) {
		return "", "", false, journalCorrupt(f.name, data.payloadOffset, "DATA payload changed while being read")
	}
	if !truncated && f.dataHash(payload) != data.hash {
		return "", "", false, journalCorrupt(f.name, data.payloadOffset, "DATA payload hash does not match its object")
	}
	return field, string(payload[valueOffset:]), true, nil
}

func validJournalFieldName(field []byte) bool {
	if len(field) == 0 || len(field) > maxJournalFieldNameSize || field[0] >= '0' && field[0] <= '9' {
		return false
	}
	for _, character := range field {
		if character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' || character == '_' {
			continue
		}
		return false
	}
	return true
}

func selectedJournalField(field string) bool {
	switch field {
	case "_HOSTNAME", "SYSLOG_IDENTIFIER", "_COMM", "_PID", "MESSAGE":
		return true
	default:
		return false
	}
}
