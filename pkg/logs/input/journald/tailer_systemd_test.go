// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package journald

import (
	"testing"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestIsWhiteListed(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	config := JournalConfig{
		ExcludeUnits: []string{"foo", "bar"},
	}
	tailer := NewTailer(config, source, nil, nil)
	err := tailer.setup()
	assert.Nil(t, err)

	assert.False(t, tailer.isWhitelisted(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "foo",
			},
		}))

	assert.False(t, tailer.isWhitelisted(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "bar",
			},
		}))

	assert.True(t, tailer.isWhitelisted(
		&sdjournal.JournalEntry{
			Fields: map[string]string{
				sdjournal.SD_JOURNAL_FIELD_SYSTEMD_UNIT: "boo",
			},
		}))
}

func TestToMessage(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	config := JournalConfig{
		ExcludeUnits: []string{"foo", "bar"},
	}
	tailer := NewTailer(config, source, nil, nil)

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
