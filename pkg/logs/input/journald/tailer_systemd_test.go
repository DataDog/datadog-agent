// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build systemd

package journald

import (
	"testing"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestShouldDropEntry(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{ExcludeUnits: []string{"foo", "bar"}})
	tailer := NewTailer(source, nil)
	err := tailer.setup()
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
}

func TestApplicationName(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "foo", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
			},
		}))

	assert.Equal(t, "foo.service", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:         "foo.sh",
			},
		}))

	assert.Equal(t, "foo.sh", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_COMM: "foo.sh",
			},
		}))

	assert.Equal(t, "", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}))
}

func TestContent(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.JSONEq(t, string([]byte(`{"journald":{"_A":"foo.service"},"message":"bar"}`)), string(tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_MESSAGE: "bar",
				"_A": "foo.service",
			},
		})))

	assert.Equal(t, string([]byte(`{"journald":{"_A":"foo.service"}}`)), string(tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "foo.service",
			},
		})))

	assert.Equal(t, string([]byte(`{"journald":{}}`)), string(tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		})))
}

func TestSeverity(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

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
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	assert.Equal(t, "docker", tailer.getApplicationName(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "foo",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo.sh",
				containerIDKey:                               "bar",
			},
		}))
}
