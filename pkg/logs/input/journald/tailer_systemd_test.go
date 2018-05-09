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
	tailer := NewTailer(source, nil, nil)
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

func TestToMessageWithNormalization(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

	assert.Equal(t, []byte("{\"a\":\"1\",\"b\":\"2\",\"c\":\"3\",\"d\":\"4\"}"), tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "1",
				"B":  "2",
				"_c": "3",
				"d":  "4",
			},
		}).Content())
}

func TestToMessageWithoutNormalization(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{DisableNormalization: true})
	tailer := NewTailer(source, nil, nil)

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

	assert.Equal(t, []byte("{\"B\":\"2\",\"_A\":\"1\",\"_c\":\"3\",\"d\":\"4\"}"), tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				"_A": "1",
				"B":  "2",
				"_c": "3",
				"d":  "4",
			},
		}).Content())
}

func TestHostnameGetsDeletedFromMessage(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

	assert.Equal(t, []byte("{}"), tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_HOSTNAME: "foo",
			},
		}).Content())
}

func TestServiceValue(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil, nil)

	var err error
	err = tailer.setup()
	assert.Nil(t, err)
	err = tailer.seek("")
	assert.Nil(t, err)

	assert.Equal(t, "kernel", tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "kernel",
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT:      "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:              "foo",
			},
		}).GetOrigin().Service)

	assert.Equal(t, "foo.service", tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo.service",
				sdjournal.SD_JOURNAL_FIELD_COMM:         "foo",
			},
		}).GetOrigin().Service)

	assert.Equal(t, "foo", tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_COMM: "foo",
			},
		}).GetOrigin().Service)

	assert.Equal(t, "", tailer.toMessage(
		&sdjournal.JournalEntry{
			Fields: map[string]string{},
		}).GetOrigin().Service)
}
