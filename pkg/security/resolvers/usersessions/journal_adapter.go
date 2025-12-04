// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package usersessions

import (
	"time"

	"github.com/coreos/go-systemd/sdjournal"
)

// SDJournalAdapter adapts sdjournal.Journal to our Journal interface
type SDJournalAdapter struct {
	journal *sdjournal.Journal
}

// NewSDJournalAdapter creates a new SDJournalAdapter
func NewSDJournalAdapter() (*SDJournalAdapter, error) {
	j, err := sdjournal.NewJournal()
	if err != nil {
		return nil, err
	}
	return &SDJournalAdapter{journal: j}, nil
}

// AddMatch adds a filter to the journal
func (s *SDJournalAdapter) AddMatch(match string) error {
	return s.journal.AddMatch(match)
}

// Close closes the journal
func (s *SDJournalAdapter) Close() error {
	return s.journal.Close()
}

// GetCursor returns the current cursor
func (s *SDJournalAdapter) GetCursor() (string, error) {
	return s.journal.GetCursor()
}

// GetEntry returns the current entry
func (s *SDJournalAdapter) GetEntry() (*JournalEntry, error) {
	entry, err := s.journal.GetEntry()
	if err != nil {
		return nil, err
	}

	return &JournalEntry{
		Fields:             entry.Fields,
		RealtimeTimestamp:  entry.RealtimeTimestamp,
		MonotonicTimestamp: entry.MonotonicTimestamp,
	}, nil
}

// Next go to the next entry
func (s *SDJournalAdapter) Next() (uint64, error) {
	return s.journal.Next()
}

// NextSkip skip n entries
func (s *SDJournalAdapter) NextSkip(skip uint64) (uint64, error) {
	return s.journal.NextSkip(skip)
}

// SeekCursor set the journal to a specific cursor
func (s *SDJournalAdapter) SeekCursor(cursor string) error {
	return s.journal.SeekCursor(cursor)
}

// SeekHead set the journal to the beginning
func (s *SDJournalAdapter) SeekHead() error {
	return s.journal.SeekHead()
}

// SeekTail positionne le journal à la fin
func (s *SDJournalAdapter) SeekTail() error {
	return s.journal.SeekTail()
}

// Wait attend qu'il y ait de nouvelles entrées dans le journal
func (s *SDJournalAdapter) Wait(timeout time.Duration) int {
	return s.journal.Wait(timeout)
}
