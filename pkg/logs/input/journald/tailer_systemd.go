// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build systemd

package journald

import (
	"encoding/json"
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
	config     JournalConfig
	source     *config.LogSource
	outputChan chan message.Message
	journal    *sdjournal.Journal
	stop       chan struct{}
	done       chan struct{}
}

// setup configures the tailer
func (t *Tailer) setup() error {
	config := t.config
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

	for _, unit := range config.Units {
		// add filters to collect only the logs of the units defined in the configuration,
		// if no units are defined, collect all the logs of the journal by default.
		err := t.journal.AddMatch("unit=" + unit)
		if err != nil {
			return err
		}
	}

	return nil
}

// seek seeks to the cursor if it is not empty or the end of the journal,
// returns an error if the operation failed.
func (t *Tailer) seek(cursor string) error {
	if cursor != "" {
		return t.journal.SeekCursor(cursor)
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
				err := fmt.Errorf("Cant't tail journal: %s", err)
				dt.source.Status.Error(err)
				log.Error(err)
				return
			}
			if n < 1 {
				// no new entry
				t.journal.Wait(defaultWaitDuration)
				continue
			}
			entry, err := t.journal.GetEntry()
			if err != nil {
				// could not parse entry
				continue
			}
			t.outputChan <- t.toMessage(entry)
		}
	}
}

// toMessage transforms a journal entry into a message.
// A journal entry has different fields that may vary depending on its nature,
// for more information, see https://www.freedesktop.org/software/systemd/man/systemd.journal-fields.html.
func (t *Tailer) toMessage(entry *sdjournal.JournalEntry) message.Message {
	payload := make(map[string]string)
	for key, value := range entry.Fields {
		// clean all keys
		key = strings.TrimLeft(key, "_")
		key = strings.ToLower(key)
		payload[key] = value
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
