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
)

func TestIsContainerEntry(t *testing.T) {
	var entry *sdjournal.JournalEntry

	entry = &sdjournal.JournalEntry{
		Fields: map[string]string{
			containerIDKey: "0123456789",
		},
	}
	assert.True(t, isContainerEntry(entry))

	entry = &sdjournal.JournalEntry{}
	assert.False(t, isContainerEntry(entry))
}
