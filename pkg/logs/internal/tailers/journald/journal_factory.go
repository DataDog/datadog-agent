//go:build systemd
// +build systemd

package journald

import (
	"time"

	"github.com/coreos/go-systemd/sdjournal"
)

// Journal interfacae to wrap the functions defined in sdjournal.
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
	// NewJournal creates a new jounral instance or error
	NewJournal() (Journal, error)

	// NewJournal creates a new jounral instance from the supplied path or error
	NewJournalFromPath(path string) (Journal, error)
}

// SDJournalFactory a JounralFactory implementation that produces sdjournal instances
type SDJournalFactory struct{}

func (s *SDJournalFactory) NewJournal() (Journal, error) {
	return sdjournal.NewJournal()
}

func (s *SDJournalFactory) NewJournalFromPath(path string) (Journal, error) {
	return sdjournal.NewJournalFromDir(path)
}
