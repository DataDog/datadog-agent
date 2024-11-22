// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

//nolint:revive // TODO(AML) Fix revive linter
package journald

import (
	"github.com/coreos/go-systemd/sdjournal"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// containerIDKey represents the key of the container identifier in a journal entry.
const containerIDKey = "CONTAINER_ID_FULL"

// isContainerEntry returns true if the entry comes from a docker container.
func (t *Tailer) isContainerEntry(entry *sdjournal.JournalEntry) bool {
	_, exists := entry.Fields[containerIDKey]
	return exists
}

// getContainerID returns the container identifier of the journal entry.
func (t *Tailer) getContainerID(entry *sdjournal.JournalEntry) string {
	//nolint:gosimple // TODO(AML) Fix gosimple linter
	containerID, _ := entry.Fields[containerIDKey]
	return containerID
}

// getContainerTags returns all the tags of a given container.
func (t *Tailer) getContainerTags(containerID string) []string {
	tags, err := t.tagger.Tag(types.NewEntityID(types.ContainerID, containerID), types.HighCardinality)
	if err != nil {
		log.Warn(err)
	}
	return tags
}
