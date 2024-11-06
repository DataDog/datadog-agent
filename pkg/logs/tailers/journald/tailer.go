// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/coreos/go-systemd/sdjournal"

	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/tag"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultWaitDuration represents the delay before which we try to collect a new log from the journal
const (
	defaultWaitDuration    = 1 * time.Second
	defaultApplicationName = "docker"
)

// Tailer collects logs from a journal.
type Tailer struct {
	decoder    *decoder.Decoder
	source     *sources.LogSource
	outputChan chan *message.Message
	journal    Journal
	exclude    struct {
		systemUnits map[string]bool
		userUnits   map[string]bool
		matches     map[string]map[string]bool
	}
	stop chan struct{}
	done chan struct{}
	// processRawMessage indicates if we want to process and send the whole structured log message
	// instead of on the logs content.
	processRawMessage bool

	// tagProvider provides additional tags to be attached to each log message.  It
	// is called once for each log message.
	tagProvider tag.Provider
	tagger      tagger.Component
}

// NewTailer returns a new tailer.
func NewTailer(source *sources.LogSource, outputChan chan *message.Message, journal Journal, processRawMessage bool, tagger tagger.Component) *Tailer {
	if len(source.Config.ProcessingRules) > 0 && processRawMessage {
		log.Warn("The logs processing rules currently apply to the raw journald JSON-structured log. These rules can now be applied to the message content only, and we plan to make this the default behavior in the future.")
		log.Warn("In order to immediately switch to this new behavior, set 'process_raw_message' to 'false' in your logs integration config and adapt your processing rules accordingly.")
		log.Warn("Please contact Datadog support for more information.")
		telemetry.GetStatsTelemetryProvider().Gauge(processor.UnstructuredProcessingMetricName, 1, []string{"tailer:journald"})
	}

	return &Tailer{
		decoder:           decoder.NewDecoderWithFraming(sources.NewReplaceableSource(source), noop.New(), framer.NoFraming, nil, status.NewInfoRegistry()),
		source:            source,
		outputChan:        outputChan,
		journal:           journal,
		stop:              make(chan struct{}, 1),
		done:              make(chan struct{}, 1),
		processRawMessage: processRawMessage,
		tagProvider:       tag.NewLocalProvider([]string{}),
		tagger:            tagger,
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
	t.source.AddInput(t.Identifier())
	log.Info("Start tailing journal ", t.journalPath(), " with id: ", t.Identifier())

	go t.forwardMessages()
	t.decoder.Start()
	go t.tail()

	return nil
}

// Stop stops the tailer
func (t *Tailer) Stop() {
	log.Info("Stop tailing journal ", t.journalPath(), " with id: ", t.Identifier())

	// stop the tail() routine
	t.stop <- struct{}{}

	t.source.RemoveInput(t.Identifier())

	<-t.done
}

// setup configures the tailer
func (t *Tailer) setup() error {
	config := t.source.Config

	matchRe := regexp.MustCompile("^([^=]+)=(.+)$")

	// add filters to collect only the logs of the units defined in the configuration,
	// if no units for both System and User, and no matches are defined,
	// collect all the logs of the journal by default.
	for _, unit := range config.IncludeSystemUnits {
		// add filters to collect only the logs of the system-level units defined in the configuration.
		match := sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT + "=" + unit
		err := t.journal.AddMatch(match)
		if err != nil {
			return fmt.Errorf("could not add filter %s: %s", match, err)
		}
	}

	if len(config.IncludeSystemUnits) > 0 && len(config.IncludeUserUnits) > 0 {
		// add Logical OR if both System and User include filters are used.
		err := t.journal.AddDisjunction()
		if err != nil {
			return fmt.Errorf("could not logical OR in the match list: %s", err)
		}
	}

	for _, unit := range config.IncludeUserUnits {
		// add filters to collect only the logs of the user-level units defined in the configuration.
		match := sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT + "=" + unit
		err := t.journal.AddMatch(match)
		if err != nil {
			return fmt.Errorf("could not add filter %s: %s", match, err)
		}
	}

	for _, match := range config.IncludeMatches {
		// add filters to collect only the logs of the matches defined in the configuration.
		submatches := matchRe.FindStringSubmatch(match)
		if len(submatches) < 1 {
			return fmt.Errorf("incorrectly formatted IncludeMatch (must be `[field]=[value]`: %s", match)
		}
		err := t.journal.AddMatch(match)
		if err != nil {
			return fmt.Errorf("could not add filter %s: %s", match, err)
		}
	}

	t.exclude.systemUnits = make(map[string]bool)
	for _, unit := range config.ExcludeSystemUnits {
		// add filters to drop all the logs related to system units to exclude.
		t.exclude.systemUnits[unit] = true
	}

	t.exclude.userUnits = make(map[string]bool)
	for _, unit := range config.ExcludeUserUnits {
		// add filters to drop all the logs related to user units to exclude.
		t.exclude.userUnits[unit] = true
	}

	t.exclude.matches = make(map[string]map[string]bool)
	for _, match := range config.ExcludeMatches {
		// add filters to drop all the logs related to the matches to exclude.
		submatches := matchRe.FindStringSubmatch(match)
		if len(submatches) < 1 {
			return fmt.Errorf("incorrectly formatted ExcludeMatch (must be `[field]=[value]`: %s", match)
		}

		key := submatches[1]
		if t.exclude.matches[key] == nil {
			t.exclude.matches[key] = map[string]bool{}
		}
		value := submatches[2]
		t.exclude.matches[key][value] = true
	}

	return nil
}

func (t *Tailer) forwardMessages() {
	for decodedMessage := range t.decoder.OutputChan {
		if len(decodedMessage.GetContent()) > 0 {
			t.outputChan <- decodedMessage
		}
	}
}

// seek seeks to the cursor if it is not empty or the end of the journal,
// returns an error if the operation failed.
func (t *Tailer) seek(cursor string) error {
	mode, _ := config.TailingModeFromString(t.source.Config.TailingMode)

	seekHead := func() error {
		if err := t.journal.SeekHead(); err != nil {
			return err
		}
		_, err := t.journal.Next() // SeekHead must be followed by Next
		return err
	}
	seekTail := func() error {
		if err := t.journal.SeekTail(); err != nil {
			return err
		}
		_, err := t.journal.Previous() // SeekTail must be followed by Previous
		return err
	}

	if mode == config.ForceBeginning {
		return seekHead()
	}
	if mode == config.ForceEnd {
		return seekTail()
	}

	// If a position is not forced from the config, try the cursor
	if cursor != "" {
		err := t.journal.SeekCursor(cursor)
		if err != nil {
			return err
		}
		// must skip one entry since the cursor points to the last committed one.
		_, err = t.journal.NextSkip(1)
		return err
	}

	// If there is no cursor and an option is not forced, use the config setting
	if mode == config.Beginning {
		return seekHead()
	}
	return seekTail()
}

// tail tails the journal until a message stop is received.
func (t *Tailer) tail() {
	defer func() {
		t.journal.Close()
		t.decoder.Stop()
		t.done <- struct{}{}
	}()
	for {
		select {
		case <-t.stop:
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

			structuredContent, jsonMarshaled := t.getContent(entry)
			var msg *message.Message
			if t.processRawMessage {
				msg = message.NewMessage(
					jsonMarshaled,
					t.getOrigin(entry),
					t.getStatus(entry),
					time.Now().UnixNano(),
				)
			} else {
				msg = message.NewStructuredMessage(
					&structuredContent,
					t.getOrigin(entry),
					t.getStatus(entry),
					time.Now().UnixNano(),
				)
			}

			select {
			case <-t.stop:
				return
			case t.decoder.InputChan <- msg:
			}
		}
	}
}

// shouldDrop returns true if the entry should be dropped,
// returns false otherwise.
func (t *Tailer) shouldDrop(entry *sdjournal.JournalEntry) bool {
	for key, values := range t.exclude.matches {
		if value, ok := entry.Fields[key]; ok {
			if _, contains := values[value]; contains {
				return true
			}
		}
	}

	sysUnit, exists := entry.Fields[sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT]
	if !exists {
		return false
	}
	usrUnit, exists := entry.Fields[sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT]
	if !exists {
		// JournalEntry is a System-level unit
		excludeAllSys := t.exclude.systemUnits["*"]
		if _, excluded := t.exclude.systemUnits[sysUnit]; excludeAllSys || excluded {
			// drop the entry
			return true
		}
	} else {
		// JournalEntry is a User-level unit
		excludeAllUsr := t.exclude.userUnits["*"]
		if _, excluded := t.exclude.userUnits[usrUnit]; excludeAllUsr || excluded {
			// drop the entry
			return true
		}
	}
	return false
}

// getContent transforms the given journal entry into two usable data struct:
// one being a structured log message, the second one being the old marshaled
// format used for an unstrucutred message.
// Note that for the former, we would not need to marshal the data, but it still
// needed for now to compute the amount of bytes read for the source telemetry.
//
// In the marshaled data, "MESSAGE" is remapped into "message" and
// all the other keys are accessible in a "journald" attribute.
// ex:
//   - journal-entry:
//     {
//     "MESSAGE": "foo",
//     "_SYSTEMD_UNIT": "foo",
//     ...
//     }
//   - message-content:
//     {
//     "message": "foo",
//     "journald": {
//     "_SYSTEMD_UNIT": "foo",
//     ...
//     }
//     }
//
// This method is modifying the original map from the entry to avoid a copy.
func (t *Tailer) getContent(entry *sdjournal.JournalEntry) (message.BasicStructuredContent, []byte) {
	payload := message.BasicStructuredContent{
		Data: make(map[string]interface{}),
	}
	fields := entry.Fields
	var msg string
	var exists bool

	// remap systemd "MESSAGE" key into the internal "message" representation
	if msg, exists = fields[sdjournal.SD_JOURNAL_FIELD_MESSAGE]; exists {
		payload.SetContent([]byte(msg))
		delete(fields, sdjournal.SD_JOURNAL_FIELD_MESSAGE) // remove it from the root structure
	}
	payload.Data["journald"] = fields

	jsonMarshaled, err := json.Marshal(payload.Data)
	if err != nil {
		log.Error("can't marshal journald tailed log", err)
		// if we're running with the old behavior,
		// ensure the message has some content if the json encoding failed
		if t.processRawMessage {
			jsonMarshaled = []byte(msg)
		}
	}
	t.source.BytesRead.Add(int64(len(jsonMarshaled)))
	return payload, jsonMarshaled
}

// getOrigin returns the message origin computed from the journal entry
func (t *Tailer) getOrigin(entry *sdjournal.JournalEntry) *message.Origin {
	origin := message.NewOrigin(t.source)
	origin.Identifier = t.Identifier()
	origin.Offset, _ = t.journal.GetCursor()
	// set the service and the source attributes of the message,
	// those values are still overridden by the integration config when defined
	tags := t.getTags(entry)
	applicationName := t.getApplicationName(entry, tags)
	origin.SetSource(applicationName)
	origin.SetService(applicationName)
	origin.SetTags(append(tags, t.tagProvider.GetTags()...))
	return origin
}

// applicationKeys represents all the valid attributes used to extract the value of the application name of a journal entry.
var applicationKeys = []string{
	sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER, // "SYSLOG_IDENTIFIER"
	sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT, // "_SYSTEMD_USER_UNIT"
	sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT,      // "_SYSTEMD_UNIT"
	sdjournal.SD_JOURNAL_FIELD_COMM,              // "_COMM"
}

// getApplicationName returns the name of the application from where the entry is from.
func (t *Tailer) getApplicationName(entry *sdjournal.JournalEntry, tags []string) string {
	if t.isContainerEntry(entry) {
		if t.source.Config.ContainerMode {
			if shortName, found := getDockerImageShortName(t.getContainerID(entry), tags); found {
				return shortName
			}
		}

		return defaultApplicationName
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
		tags = t.getContainerTags(t.getContainerID(entry))
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
	return Identifier(t.source.Config)
}

// Identifier returns the unique identifier of the current journald config
func Identifier(config *config.LogsConfig) string {
	id := "default"
	if config.ConfigId != "" {
		id = config.ConfigId
	} else if config.Path != "" {
		id = config.Path
	}
	return journaldIntegration + ":" + id
}

// journalPath returns the path of the journal
func (t *Tailer) journalPath() string {
	if t.source.Config.Path != "" {
		return t.source.Config.Path
	}
	return "default"
}
