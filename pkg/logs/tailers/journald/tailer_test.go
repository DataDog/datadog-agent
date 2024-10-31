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

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type MockJournal struct {
	m        *sync.Mutex
	seekTail int
	seekHead int
	next     int
	previous int
	cursor   string
	entries  []*sdjournal.JournalEntry
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) AddMatch(match string) error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) AddDisjunction() error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) SeekTail() error {
	m.seekTail++
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) SeekHead() error {
	m.seekHead++
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) Wait(timeout time.Duration) int {
	time.Sleep(time.Millisecond)
	return 0
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) SeekCursor(cursor string) error {
	m.cursor = cursor
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) NextSkip(skip uint64) (uint64, error) {
	return 0, nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) Close() error {
	return nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) Next() (uint64, error) {
	m.m.Lock()
	defer m.m.Unlock()
	m.next++
	return uint64(len(m.entries)), nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) Previous() (uint64, error) {
	m.m.Lock()
	defer m.m.Unlock()
	m.previous++
	return uint64(len(m.entries)), nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) GetEntry() (*sdjournal.JournalEntry, error) {
	m.m.Lock()
	defer m.m.Unlock()
	defer func() {
		m.entries = m.entries[1:]
	}()

	if len(m.entries) == 0 {
		return nil, nil
	}

	return m.entries[0], nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) GetCursor() (string, error) {
	return "", nil
}

func TestIdentifier(t *testing.T) {
	var tailer *Tailer
	var source *sources.LogSource
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	// expect default identifier
	source = sources.NewLogSource("", &config.LogsConfig{})
	tailer = NewTailer(source, nil, nil, true, fakeTagger)
	assert.Equal(t, "journald:default", tailer.Identifier())

	// expect identifier to be overridden
	source = sources.NewLogSource("", &config.LogsConfig{Path: "any_path"})
	tailer = NewTailer(source, nil, nil, true, fakeTagger)
	assert.Equal(t, "journald:any_path", tailer.Identifier())
}

func TestShouldDropEntry(t *testing.T) {
	// System-level service units do not have SD_JOURNAL_FIELD_SYSTEMD_USER_UNIT
	// User-level service units may have a common value for SD_JOURNAL_FIELD_SYSTEMD_UNIT
	var source *sources.LogSource
	var tailer *Tailer
	var err error
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	// expect only the specified service units or matching entries to be dropped
	source = sources.NewLogSource("", &config.LogsConfig{ExcludeSystemUnits: []string{"foo", "bar"}, ExcludeUserUnits: []string{"baz", "qux"}, ExcludeMatches: []string{"quux=quuz"}})
	tailer = NewTailer(source, nil, nil, true, fakeTagger)
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
	tailer = NewTailer(source, nil, nil, true, fakeTagger)
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
	tailer = NewTailer(source, nil, nil, true, fakeTagger)
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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

	_, marshaled := tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_MESSAGE: "bar",
				"_A":                               "foo.service",
			},
		},
	)

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"},"message":"bar"}`), marshaled)
	_, marshaled = tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "foo.service",
			},
		},
	)
	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"}}`), marshaled)

	_, marshaled = tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		},
	)
	assert.Equal(t, []byte(`{"journald":{}}`), marshaled)
}

func TestSeverity(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, nil, nil, true, fakeTagger)

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
		{"default no cursor", &config.LogsConfig{}, "", &MockJournal{m: m, seekTail: 1, previous: 1}},
		{"default has cursor", &config.LogsConfig{}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - seek head", &config.LogsConfig{TailingMode: "beginning"}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - seek tail", &config.LogsConfig{TailingMode: "end"}, "123", &MockJournal{m: m, cursor: "123"}},
		{"has cursor - force head", &config.LogsConfig{TailingMode: "forceBeginning"}, "123", &MockJournal{m: m, seekHead: 1, next: 1}},
		{"has cursor - force tail", &config.LogsConfig{TailingMode: "forceEnd"}, "123", &MockJournal{m: m, seekTail: 1, previous: 1}},
		{"no cursor - force head", &config.LogsConfig{TailingMode: "forceBeginning"}, "", &MockJournal{m: m, seekHead: 1, next: 1}},
		{"no cursor - force tail", &config.LogsConfig{TailingMode: "forceEnd"}, "", &MockJournal{m: m, seekTail: 1, previous: 1}},
		{"no cursor - seek head", &config.LogsConfig{TailingMode: "beginning"}, "", &MockJournal{m: m, seekHead: 1, next: 1}},
		{"no cursor - seek tail", &config.LogsConfig{TailingMode: "end"}, "", &MockJournal{m: m, seekTail: 1, previous: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockJournal := &MockJournal{m: m}
			source := sources.NewLogSource("", tt.config)
			fakeTagger := taggerimpl.SetupFakeTagger(t)
			tailer := NewTailer(source, nil, mockJournal, true, fakeTagger)
			tailer.Start(tt.cursor)

			mockJournal.m.Lock()
			assert.Equal(t, tt.expectedJournalState.cursor, mockJournal.cursor)

			// .Next() is called again by the tail goroutine, so expect it to be equal or greater than expected.
			assert.True(t, tt.expectedJournalState.next <= mockJournal.next)
			assert.Equal(t, tt.expectedJournalState.previous, mockJournal.previous)
			assert.Equal(t, tt.expectedJournalState.seekHead, mockJournal.seekHead)
			assert.Equal(t, tt.expectedJournalState.seekTail, mockJournal.seekTail)
			assert.Equal(t, tt.expectedJournalState.entries, mockJournal.entries)

			mockJournal.m.Unlock()

			tailer.Stop()
		})
	}
}

func TestTailerCanTailJournal(t *testing.T) {

	mockJournal := &MockJournal{m: &sync.Mutex{}}
	source := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, make(chan *message.Message, 1), mockJournal, true, fakeTagger)

	mockJournal.entries = append(mockJournal.entries, &sdjournal.JournalEntry{Fields: map[string]string{"MESSAGE": "foobar"}})

	tailer.Start("")

	resultMessage := <-tailer.outputChan

	var parsedContent map[string]interface{}
	json.Unmarshal(resultMessage.GetContent(), &parsedContent)
	assert.Equal(t, "foobar", parsedContent["message"])

	tailer.Stop()
}

func TestTailerWithStructuredMessage(t *testing.T) {
	assert := assert.New(t)

	mockJournal := &MockJournal{m: &sync.Mutex{}}
	source := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailer := NewTailer(source, make(chan *message.Message, 1), mockJournal, false, fakeTagger)
	mockJournal.entries = append(mockJournal.entries, &sdjournal.JournalEntry{Fields: map[string]string{
		sdjournal.SD_JOURNAL_FIELD_MESSAGE: "foobar",
		"_SESSION_UID":                     "a97aaca9-ea7a-4ea5-9ebe-048686f2c78a",
	}})

	tailer.Start("")
	defer tailer.Stop()

	resultMessage := <-tailer.outputChan
	assert.Equal([]byte("foobar"), resultMessage.GetContent())

	data, err := resultMessage.Render()
	assert.NoError(err)
	assert.Equal(data, []byte("{\"journald\":{\"_SESSION_UID\":\"a97aaca9-ea7a-4ea5-9ebe-048686f2c78a\"},\"message\":\"foobar\"}"))
}

func TestTailerCompareUnstructuredAndStructured(t *testing.T) {
	assert := assert.New(t)

	// v1 behavior tailer

	mockJournalV1 := &MockJournal{m: &sync.Mutex{}}
	sourceV1 := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerimpl.SetupFakeTagger(t)
	tailerV1 := NewTailer(sourceV1, make(chan *message.Message, 1), mockJournalV1, true, fakeTagger)
	mockJournalV1.entries = append(mockJournalV1.entries, &sdjournal.JournalEntry{Fields: map[string]string{
		sdjournal.SD_JOURNAL_FIELD_MESSAGE: "journald log message content",
		"_SESSION_UID":                     "a97aaca9-ea7a-4ea5-9ebe-048686f2c78a",
	}})

	tailerV1.Start("")
	defer tailerV1.Stop()

	// v2 behavior tailer

	mockJournalV2 := &MockJournal{m: &sync.Mutex{}}
	sourceV2 := sources.NewLogSource("", &config.LogsConfig{})
	tailerV2 := NewTailer(sourceV2, make(chan *message.Message, 1), mockJournalV2, false, fakeTagger)
	mockJournalV2.entries = append(mockJournalV2.entries, &sdjournal.JournalEntry{Fields: map[string]string{
		sdjournal.SD_JOURNAL_FIELD_MESSAGE: "journald log message content",
		"_SESSION_UID":                     "a97aaca9-ea7a-4ea5-9ebe-048686f2c78a",
	}})

	tailerV2.Start("")
	defer tailerV2.Stop()

	// render both, we should get the same content
	resultMessageV1 := <-tailerV1.outputChan
	resultMessageV2 := <-tailerV2.outputChan

	v1, err1 := resultMessageV1.Render()
	v2, err2 := resultMessageV2.Render()
	assert.NoError(err1)
	assert.NoError(err2)

	assert.Equal(v1, v2)
}

func TestExpectedTagDuration(t *testing.T) {

	mockConfig := configmock.New(t)

	tags := []string{"tag1:value1"}
	fakeTagger := taggerimpl.SetupFakeTagger(t)

	mockConfig.SetWithoutSource("tags", tags)
	defer mockConfig.SetWithoutSource("tags", nil)

	mockConfig.SetWithoutSource("logs_config.expected_tags_duration", "5s")
	defer mockConfig.SetWithoutSource("logs_config.expected_tags_duration", "0")

	mockJournal := &MockJournal{m: &sync.Mutex{}}
	source := sources.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, make(chan *message.Message, 1), mockJournal, true, fakeTagger)

	mockJournal.entries = append(mockJournal.entries, &sdjournal.JournalEntry{Fields: map[string]string{"MESSAGE": "foobar"}})

	tailer.Start("")
	assert.Equal(t, tags, (<-tailer.outputChan).Tags())

	tailer.Stop()

}
