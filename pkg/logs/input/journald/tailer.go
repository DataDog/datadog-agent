// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build systemd

package journald

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/coreos/go-systemd/sdjournal"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultWaitDuration represents the delay before which we try to collect a new log from the journal
const defaultWaitDuration = 1 * time.Second

// Tailer collects logs from a journal.
type Tailer struct {
	source     *config.LogSource
	outputChan chan *message.Message
	journal    *sdjournal.Journal
	blacklist  map[string]bool
	stop       chan struct{}
	done       chan struct{}
}

// NewTailer returns a new tailer.
func NewTailer(source *config.LogSource, outputChan chan *message.Message) *Tailer {
	return &Tailer{
		source:     source,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
		done:       make(chan struct{}, 1),
	}
}

// Start starts tailing the journal from a given offset.
func (t *Tailer) Start(cursor string) error {
	if err := t.setup(); err != nil {
		t.source.Status.Error(err)
		return err
	}
	if err := t.seek(cursor); err != nil {
		t.source.Status.Error(err)
		return err
	}
	t.source.Status.Success()
	t.source.AddInput(t.journalPath())
	log.Info("Start tailing journal ", t.journalPath())
	go t.tail()
	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing journal ", t.journalPath())
	t.stop <- struct{}{}
	t.source.RemoveInput(t.journalPath())
	<-t.done
}

// setup configures the tailer
func (t *Tailer) setup() error {
	config := t.source.Config
	var err error

	t.initializeTagger()

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

	t.blacklist = make(map[string]bool)
	for _, unit := range config.ExcludeUnits {
		// add filters to drop all the logs related to units to exclude.
		t.blacklist[unit] = true
	}

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
				err := fmt.Errorf("cant't tail journal %s: %s", t.journalPath(), err)
				t.source.Status.Error(err)
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

// shouldDrop returns true if the entry should be dropped,
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
func (t *Tailer) toMessage(entry *sdjournal.JournalEntry) *message.Message {
	return message.NewPartialMessage3(t.getContent(entry), t.getOrigin(entry), t.getStatus(entry))
}

// getContent returns all the fields of the entry as a json-string,
// remapping "MESSAGE" into "message" and bundling all the other keys in a "journald" attribute.
// ex:
// * journal-entry:
//  {
//    "MESSAGE": "foo",
//    "_SYSTEMD_UNIT": "foo",
//    ...
//  }
// * message-content:
//  {
//    "message": "foo",
//    "journald": {
//      "_SYSTEMD_UNIT": "foo",
//      ...
//    }
//  }
func (t *Tailer) getContent(entry *sdjournal.JournalEntry) []byte {
	payload := make(map[string]interface{})
	fields := entry.Fields
	if message, exists := fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]; exists {
		payload["message"] = message
		delete(fields, sdjournal.SD_JOURNAL_FIELD_MESSAGE)
	}
	payload["journald"] = fields

	content, err := json.Marshal(payload)
	if err != nil {
		// ensure the message has some content if the json encoding failed
		value, _ := entry.Fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]
		content = []byte(value)
	}

	return content
}

// getOrigin returns the message origin computed from the journal entry
func (t *Tailer) getOrigin(entry *sdjournal.JournalEntry) *message.Origin {
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.Offset, _ = t.journal.GetCursor()
	// set the service and the source attributes of the message,
	// those values are still overridden by the integration config when defined
	applicationName := t.getApplicationName(entry)
	origin.SetSource(applicationName)
	origin.SetService(applicationName)
	origin.SetTags(t.getTags(entry))
	return origin
}

// applicationKeys represents all the valid attributes used to extract the value of the application name of a journal entry.
var applicationKeys = []string{
	sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER, // "SYSLOG_IDENTIFIER"
	sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT,      // "_SYSTEMD_UNIT"
	sdjournal.SD_JOURNAL_FIELD_COMM,              // "_COMM"
}

// getApplicationName returns the name of the application from where the entry is from.
func (t *Tailer) getApplicationName(entry *sdjournal.JournalEntry) string {
	if t.isContainerEntry(entry) {
		return "docker"
	}
	for _, key := range applicationKeys {
		if value, exists := entry.Fields[key]; exists {
			return value
		}
	}
	return ""
}

// getTags returns a list of tags matching with the journal entry.
func (t *Tailer) getTags(entry *sdjournal.JournalEntry) []string {
	var tags []string
	if t.isContainerEntry(entry) {
		tags = append(tags, t.getContainerTags(t.getContainerID(entry))...)
	}
	return tags
}

// priorityStatusMapping represents the 1:1 mapping between journal entry priorities and statuses.
var priorityStatusMapping = map[string]string{
	"0": message.StatusEmergency,
	"1": message.StatusAlert,
	"2": message.StatusCritical,
	"3": message.StatusError,
	"4": message.StatusWarning,
	"5": message.StatusNotice,
	"6": message.StatusInfo,
	"7": message.StatusDebug,
}

// getStatus returns the status of the journal entry,
// returns "info" by default if no valid value is found.
func (t *Tailer) getStatus(entry *sdjournal.JournalEntry) string {
	priority, exists := entry.Fields[sdjournal.SD_JOURNAL_FIELD_PRIORITY]
	if !exists {
		return message.StatusInfo
	}
	status, exists := priorityStatusMapping[priority]
	if !exists {
		return message.StatusInfo
	}
	return status
}

// journaldIntegration represents the name of the integration,
// it's used to override the source of the message and as a fingerprint to store the journal cursor.
const journaldIntegration = "journald"

// Identifier returns the unique identifier of the current journal being tailed.
func (t *Tailer) Identifier() string {
	return journaldIntegration + ":" + t.journalPath()
}

// journalPath returns the path of the journal
func (t *Tailer) journalPath() string {
	if t.source.Config.Path != "" {
		return t.source.Config.Path
	}
	return "default"
}
