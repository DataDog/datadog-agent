// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"context"

	"github.com/coreos/go-systemd/sdjournal"

	"github.com/DataDog/datadog-agent/pkg/tagger"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
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
	containerID, _ := entry.Fields[containerIDKey]
	return containerID
}

// getContainerTags returns all the tags of a given container.
func (t *Tailer) getContainerTags(containerID string) []string {
	tags, err := tagger.Tag(containers.BuildTaggerEntityName(containerID), collectors.HighCardinality)
	if err != nil {
		log.Warn(err)
	}
	return tags
}

// initializeTagger initializes the tag collector.
func (t *Tailer) initializeTagger() {
	err := tagger.Init(context.TODO())
	if err != nil {
		log.Errorf("failed to start the tagger: %s", err)
	}
}
