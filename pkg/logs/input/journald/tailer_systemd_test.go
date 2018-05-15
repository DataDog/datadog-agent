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

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

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

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

	assert.Equal(t, []byte(`{"journald":{"_A":"foo.service"},"message":"bar"}`), tailer.getContent(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_MESSAGE: "bar",
				"_A": "foo.service",
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
