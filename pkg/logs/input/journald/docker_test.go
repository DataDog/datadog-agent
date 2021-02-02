// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build systemd

package journald

import (
	"testing"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestIsContainerEntry(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	var entry *sdjournal.JournalEntry

	entry = &sdjournal.JournalEntry{
		Fields: map[string]string{
			containerIDKey: "0123456789",
		},
	}
	assert.True(t, tailer.isContainerEntry(entry))

	entry = &sdjournal.JournalEntry{}
	assert.False(t, tailer.isContainerEntry(entry))
}

func TestGetContainerID(t *testing.T) {
	source := config.NewLogSource("", &config.LogsConfig{})
	tailer := NewTailer(source, nil)

	entry := &sdjournal.JournalEntry{
		Fields: map[string]string{
			containerIDKey: "0123456789",
		},
	}
	assert.Equal(t, "0123456789", tailer.getContainerID(entry))
}
