// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build systemd

package journald

import (
	"github.com/coreos/go-systemd/sdjournal"
)

// containerIDKey represents the key of the container identifier in a journal entry.
const containerIDKey = "CONTAINER_ID_FULL"

// isContainerEntry returns true if the entry comes from a docker container.
func (t *Tailer) isContainerEntry(entry *sdjournal.JournalEntry) bool {
	_, exists := entry.Fields[containerIDKey]
	return exists
}
