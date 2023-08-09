// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type MockJournal struct {
	m        *sync.Mutex
	seekTail int
	seekHead int
	cursor   string
	entry    *sdjournal.JournalEntry
	next     uint64
}

func (m *MockJournal) AddMatch(match string) error {
	return nil
}
func (m *MockJournal) AddDisjunction() error {
	return nil
}
func (m *MockJournal) SeekTail() error {
	m.seekTail++
	return nil
}
func (m *MockJournal) SeekHead() error {
	m.seekHead++
	return nil
}
func (m *MockJournal) Wait(timeout time.Duration) int {
	return 0
}
func (m *MockJournal) SeekCursor(cursor string) error {
	m.cursor = cursor
	return nil
}
func (m *MockJournal) NextSkip(skip uint64) (uint64, error) {
	return 0, nil
}
func (m *MockJournal) Close() error {
	return nil
}
func (m *MockJournal) Next() (uint64, error) {
	m.m.Lock()
	defer m.m.Unlock()
	return m.next, nil
}
func (m *MockJournal) GetEntry() (*sdjournal.JournalEntry, error) {
	m.m.Lock()
	defer m.m.Unlock()
	return m.entry, nil
}
func (m *MockJournal) GetCursor() (string, error) {
	return "", nil
}

func TestIdentifier(t *testing.T) {
	var tailer *Tailer
	var source *sources.LogSource

	// expect default identifier
	source = sources.NewLogSource("", &config.LogsConfig{})
	tailer = NewTailer(source, nil, nil)
	assert.Equal(t, "journald:default", tailer.Identifier())

	// expect identifier to be overidden
	source = sources.NewLogSource("", &config.LogsConfig{Path: "any_path"})
	tailer = NewTailer(source, nil, nil)
	assert.Equal(t, "journald:any_path", tailer.Identifier())
}

func TestShouldDropEntry(t *testing.T) {
	// System-level service units do not have SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT
	// User-level service units may have a common value for SD_JOURNAL_FIELD_SYSTEMD_UNIT
	var source *sources.LogSource
	var tailer *Tailer
	var err error

	// expect only the specified service units or matching entries to be dropped
	source = sources.NewLogSource("", &config.LogsConfig{ExcludeSystemUnits: []string{"foo", "bar"}, ExcludeUserUnits: []string{"baz", "qux"}, ExcludeMatches: []string{"quux=quuz"}})
	tailer = NewTailer(source, nil, nil)
	err = tailer.setup()
	assert.Nil(t, err)

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "bar",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "boo",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "bar",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "baz",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "qux",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"quux": "quuz",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"quux": "corge",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"grault": "garply",
			},
		}))

	// expect all System-level service units to be dropped
	source = sources.NewLogSource("", &config.LogsConfig{ExcludeSystemUnits: []string{"*"}})
	tailer = NewTailer(source, nil, nil)
	err = tailer.setup()
	assert.Nil(t, err)

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "bar",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "bar",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "baz",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	// expect all User-level service units to be dropped
	source = sources.NewLogSource("", &config.LogsConfig{ExcludeUserUnits: []string{"*"}})
	tailer = NewTailer(source, nil, nil)
	err = tailer.setup()
	assert.Nil(t, err)

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo",
			},
		}))

	assert.False(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "bar",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "bar",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

	assert.True(t, tailer.shouldDrop(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "baz",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "user@1000.service",
			},
		}))

}

func TestApplicationName(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, "foo", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "foo-user.service", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "foo.service", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:         "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "foo.sh", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_COMM: "foo.sh",
			},
		}, []string{}))

	assert.Equal(t, "", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}, []string{}))
}

func TestContent(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"},"message":"bar"}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_MESSAGE: "bar",
				"_A":                               "foo.service",
			},
		}))

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"}}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "foo.service",
			},
		}))

	assert.Equal(t, []byte(`{"journald":{}}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}))
}

func TestSeverity(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	priorityValues := []string{"0", "1", "2", "3", "4", "5", "6", "7", "foo"}
	statuses := []string{message.StatusEmergency, message.StatusAlert, message.StatusCritical, message.StatusError, message.StatusWarning, message.StatusNotice, message.StatusInfo, message.StatusDebug, message.StatusInfo}

	for i, priority := range priorityValues {
		assert.Equal(t, statuses[i], tailer.getStatus(
			&sdjournal.JournalEntry{
				Fields: map[string]string{
					sdjournal.SD_JOURNAL_FIELD_PRIORITY: priority,
				},
			}))
	}
}

func TestApplicationNameShouldBeDockerForContainerEntries(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, "docker", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               "bar",
			},
		}, []string{}))
}

func TestApplicationNameShouldBeShortImageForContainerEntries(t *testing.T) {
	containerID := "bar"

	source := sources.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, "testImage", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"short_image:testImage"}))

	// Verify we have the value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.True(t, hit)
}

func TestApplicationNameShouldBeDockerWhenTagNotFound(t *testing.T) {
	containerID := "bar2"

	source := sources.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, "docker", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"not_short_image:testImage"}))

	// Verify we don't have value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.False(t, hit)
}

func TestWrongTypeFromCache(t *testing.T) {
	containerID := "bar3"

	// Store wrong type in cache, verify we ignore the value
	cache.Cache.Set(getImageCacheKey(containerID), 10, 30*time.Second)

	source := sources.NewLogSource("", &config.LogsConfig{ContainerMode: true})
	tailer := NewTailer(source, nil, nil)

	assert.Equal(t, "testImage", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT: "foo-user.service",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               containerID,
			},
		}, []string{"short_image:testImage"}))

	// Verify we have the value in our cache
	_, hit := cache.Cache.Get(getImageCacheKey(containerID))
	assert.True(t, hit)
}

func TestTailingMode(t *testing.T) {
	m := &sync.Mutex{}

	tests := []struct {
		name                 string
		config               *config.LogsConfig
		cursor               string
		expectedJournalState *MockJournal
	}{
		{"default no cursor", &config.LogsConfig{}, "", &MockJournal{m: m, seekTail: 1}},
		{"default has cursor", &config.LogsConfig{}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - seek head", &config.LogsConfig{TailingMode: "beginning"}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - seek tail", &config.LogsConfig{TailingMode: "end"}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - force head", &config.LogsConfig{TailingMode: "forceBeginning"}, "123", &MockJournal{m: m, seekHead: 1}},
		{"has cursor - force tail", &config.LogsConfig{TailingMode: "forceEnd"}, "123", &MockJournal{m: m, seekTail: 1}},
		{"no cursor - force head", &config.LogsConfig{TailingMode: "forceBeginning"}, "", &MockJournal{m: m, seekHead: 1}},
		{"no cursor - force tail", &config.LogsConfig{TailingMode: "forceEnd"}, "", &MockJournal{m: m, seekTail: 1}},
		{"no cursor - seek head", &config.LogsConfig{TailingMode: "beginning"}, "", &MockJournal{m: m, seekHead: 1}},
		{"no cursor - seek tail", &config.LogsConfig{TailingMode: "end"}, "", &MockJournal{m: m, seekTail: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockJournal := &MockJournal{m: m}
			source := sources.NewLogSource("", tt.config)
			tailer := NewTailer(source, nil, mockJournal)
			tailer.Start(tt.cursor)

			assert.Equal(t, *tt.expectedJournalState, *mockJournal)
			tailer.Stop()
		})
	}
}

func TestTailerCanTailJournal(t *testing.T) {

	mockJournal := &MockJournal{m: &sync.Mutex{}, next: 1}
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, make(chan *message.Message, 1), mockJournal)

	mockJournal.entry = &sdjournal.JournalEntry{Fields: map[string]string{"MESSAGE": "foobar"}}

	tailer.Start("")

	resultMessage := <-tailer.outputChan

	var parsedContent map[string]interface{}
	json.Unmarshal(resultMessage.Content, &parsedContent)

	assert.Equal(t, parsedContent["message"], "foobar")
	tailer.Stop()
}
