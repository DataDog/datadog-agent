package journald

// +build libsystemd

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

// JournalConfig enables to configure the tailer:
// - Units: the units to filter on
// - Path: the path of the journal
type JournalConfig struct {
	Units []string
	Path  string
}

// Tailer collects logs from a journal.
type Tailer struct {
	config     JournalConfig
	source     *config.LogSource
	outputChan chan message.Message
	journal    *sdjournal.Journal
	stop       chan struct{}
}

// NewTailer returns a new tailer.
func NewTailer(config JournalConfig, source *config.LogSource, outputChan chan message.Message) *Tailer {
	return &Tailer{
		config:     config,
		source:     source,
		outputChan: outputChan,
		stop:       make(chan struct{}, 1),
	}
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

// Identifier returns the unique identifier of the current journal being tailed.
func (t *Tailer) Identifier() string {
	if t.config.Path != "" {
		return "journald:" + t.config.Path
	}
	return "journald:default"
}

// Start starts tailing the journal from a given offset.
func (t *Tailer) Start(cursor string) error {
	var err error
	err = t.setup()
	if err != nil {
		return err
	}
	if cursor != "" {
		err = t.journal.SeekCursor(cursor)
	} else {
		err = t.journal.SeekTail()
	}
	if err != nil {
		return err
	}
	log.Info("Start tailing journal")
	go t.tail()
	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing journal")
	t.stop <- struct{}{}
}

// tail tails the journal until a message stop is received.
func (t *Tailer) tail() {
	defer t.journal.Close()
	for {
		select {
		case <-t.stop:
			// stop tailing journal
			return
		default:
			n, err := t.journal.Next()
			if err != nil && err != io.EOF {
				log.Info("Cant't tail journal: ", err)
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
