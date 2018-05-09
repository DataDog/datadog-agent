// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build systemd

package journald

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	log "github.com/cihub/seelog"
	"github.com/coreos/go-systemd/sdjournal"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// defaultWaitDuration represents the delay before which we try to collect a new log from the journal
const defaultWaitDuration = 1 * time.Second

// Tailer collects logs from a journal.
type Tailer struct {
	source     *config.LogSource
	outputChan chan message.Message
	journal    *sdjournal.Journal
	blacklist  map[string]bool
	errHandler ErrorHandler
	stop       chan struct{}
	done       chan struct{}
}

// setup configures the tailer
func (t *Tailer) setup() error {
	config := t.source.Config
	var err error

	if config.Path == "" {
		// open the default journal
		t.journal, err = sdjournal.NewJournal()
	} else {
		t.journal, err = sdjournal.NewJournalFromDir(config.Path)
	}
	if err != nil {
		return err
	}

	for _, unit := range config.IncludeUnits {
		// add filters to collect only the logs of the units defined in the configuration,
		// if no units are defined, collect all the logs of the journal by default.
		match := sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT + "=" + unit
		err := t.journal.AddMatch(match)
		if err != nil {
			return fmt.Errorf("could not add filter %s: %s", match, err)
		}
	}

	blacklist := make(map[string]bool)
	for _, unit := range config.ExcludeUnits {
		blacklist[unit] = true
	}
	t.blacklist = blacklist

	return nil
}

// seek seeks to the cursor if it is not empty or the end of the journal,
// returns an error if the operation failed.
func (t *Tailer) seek(cursor string) error {
	if cursor != "" {
		err := t.journal.SeekCursor(cursor)
		if err != nil {
			return err
		}
		// must skip one entry since the cursor points to the last committed one.
		_, err = t.journal.NextSkip(1)
		return err
	} else {
		return t.journal.SeekTail()
	}
}

// tail tails the journal until a message stop is received.
func (t *Tailer) tail() {
	defer func() {
		t.journal.Close()
		t.done <- struct{}{}
	}()
	for {
		select {
		case <-t.stop:
			// stop tailing journal
			return
		default:
			n, err := t.journal.Next()
			if err != nil && err != io.EOF {
				t.errHandler.Handle(NewTailError(t.Identifier(), err))
				return
			}
			if n < 1 {
				// no new entry
				t.journal.Wait(defaultWaitDuration)
				continue
			}
			entry, err := t.journal.GetEntry()
			if err != nil {
				log.Warnf("Could not retrieve journal entry: %s", err)
				continue
			}
			if t.shouldDrop(entry) {
				continue
			}
			t.outputChan <- t.toMessage(entry)
		}
	}
}

// shouldDrop returns true if the entry should be forwarded,
// returns false otherwise.
func (t *Tailer) shouldDrop(entry *sdjournal.JournalEntry) bool {
	unit, exists := entry.Fields[sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT]
	if !exists {
		return false
	}
	if _, blacklisted := t.blacklist[unit]; blacklisted {
		// drop the entry
		return true
	}
	return false
}

// toMessage transforms a journal entry into a message.
// A journal entry has different fields that may vary depending on its nature,
// for more information, see https://www.freedesktop.org/software/systemd/man/systemd.journal-fields.html.
func (t *Tailer) toMessage(entry *sdjournal.JournalEntry) message.Message {
	var payload map[string]string
	if !t.source.Config.DisableNormalization {
		// clean all keys by striping all leading underscores and converting to lowercase:
		// ex: { "_A": "valueA", "_B": "valueB", "c": "valueC"} -> { "a": "valueA", "b": "valueB", "c": "valueC"}
		payload = make(map[string]string)
		for key, value := range entry.Fields {
			key = strings.TrimLeft(key, "_")
			key = strings.ToLower(key)
			payload[key] = value
		}
	} else {
		payload = entry.Fields
	}
	content, err := json.Marshal(payload)
	if err != nil {
		// ensure the message has some content if the json encoding failed
		value, _ := entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]
		content = []byte(value)
	}
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.Cursor, _ = t.journal.GetCursor()
	return message.New(content, origin, nil)
}
