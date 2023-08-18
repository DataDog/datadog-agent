// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"time"

	"github.com/coreos/go-systemd/sdjournal"
)

// Journal interface to wrap the functions defined in sdjournal.
type Journal interface {
	AddMatch(match string) error
	AddDisjunction() error
	SeekTail() error
	SeekHead() error
	Wait(timeout time.Duration) int
	SeekCursor(cursor string) error
	NextSkip(skip uint64) (uint64, error)
	Close() error
	Next() (uint64, error)
	GetEntry() (*sdjournal.JournalEntry, error)
	GetCursor() (string, error)
}

// JournalFactory interface that provides journal implementations
type JournalFactory interface {
	// NewJournal creates a new journal instance or error
	NewJournal() (Journal, error)

	// NewJournal creates a new journal instance from the supplied path or error
	NewJournalFromPath(path string) (Journal, error)
}
