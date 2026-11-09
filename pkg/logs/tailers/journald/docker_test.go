// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && systemd

package journald

import (
	"sync"
	"testing"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func TestIsContainerEntry(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeRegistry := auditorMock.NewMockAuditor()
	tailer := NewTailer(source, nil, nil, false, fakeTagger, fakeRegistry)

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

func TestGetOriginSharesSnapshotAcrossContainers(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{Tags: []string{"env:prod"}})
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "a"), "test", []string{"container_name:a"}, nil, nil, nil)
	fakeTagger.SetTags(types.NewEntityID(types.ContainerID, "b"), "test", []string{"container_name:b"}, nil, nil, nil)
	tailer := NewTailer(source, nil, &MockJournal{m: &sync.Mutex{}}, false, fakeTagger, auditorMock.NewMockAuditor())

	entry := func(id string) *sdjournal.JournalEntry {
		return &sdjournal.JournalEntry{Fields: map[string]string{containerIDKey: id, sdjournal.SD_JOURNAL_FIELD_SYSLOG_IDENTIFIER: "app"}}
	}
	a, b := tailer.getOrigin(entry("a")), tailer.getOrigin(entry("b"))
	require.NoError(t, message.CheckTagSnapshot(a))
	require.NoError(t, message.CheckTagSnapshot(b))
	sharedA, tagsA, _, _ := a.TagSnapshot()
	sharedB, tagsB, service, _ := b.TagSnapshot()
	assert.Same(t, sharedA, sharedB)
	assert.Equal(t, []string{"container_name:a"}, tagsA)
	assert.Equal(t, []string{"container_name:b"}, tagsB)
	assert.Equal(t, "docker", service)
}

func TestGetContainerID(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeRegistry := auditorMock.NewMockAuditor()
	tailer := NewTailer(source, nil, nil, false, fakeTagger, fakeRegistry)

	entry := &sdjournal.JournalEntry{
		Fields: map[string]string{
			containerIDKey: "0123456789",
		},
	}
	assert.Equal(t, "0123456789", tailer.getContainerID(entry))
}
