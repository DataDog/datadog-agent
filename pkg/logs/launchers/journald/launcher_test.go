// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

// Package journald provides journald-based log launchers (no-op for non-systemd builds)
package journald

import (
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/sdjournal"
	"github.com/stretchr/testify/assert"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/logs-library/pipeline"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	auditorMock "github.com/DataDog/datadog-agent/comp/logs/auditor/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/journald"
)

type MockJournal struct{}

func (m *MockJournal) AddMatch(_ string) error                    { return nil }
func (m *MockJournal) AddDisjunction() error                      { return nil }
func (m *MockJournal) SeekTail() error                            { return nil }
func (m *MockJournal) SeekHead() error                            { return nil }
func (m *MockJournal) Wait(_ time.Duration) int                   { return 0 }
func (m *MockJournal) SeekCursor(_ string) error                  { return nil }
func (m *MockJournal) NextSkip(_ uint64) (uint64, error)          { return 0, nil }
func (m *MockJournal) Close() error                               { return nil }
func (m *MockJournal) Next() (uint64, error)                      { return 0, nil }
func (m *MockJournal) Previous() (uint64, error)                  { return 0, nil }
func (m *MockJournal) GetEntry() (*sdjournal.JournalEntry, error) { return nil, sdjournal.ErrExpired }
func (m *MockJournal) GetCursor() (string, error)                 { return "", nil }

// MockJournalFactory a journal factory that produces mock journal implementations
type MockJournalFactory struct{}

func (s *MockJournalFactory) NewJournal() (tailer.Journal, error) {
	return &MockJournal{}, nil
}

func (s *MockJournalFactory) NewJournalFromPath(_ string) (tailer.Journal, error) {
	return &MockJournal{}, nil
}

func newTestLauncher(t *testing.T) *Launcher {
	t.Helper()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	launcher := NewLauncherWithFactory(&MockJournalFactory{}, flare.NewFlareController(), fakeTagger)
	launcher.Start(launchers.NewMockSourceProvider(), pipeline.NewMockProvider(), auditorMock.NewMockRegistry(), tailers.NewTailerTracker())
	return launcher
}

func TestSingeJournaldConfig(t *testing.T) {
	launcher := newTestLauncher(t)

	sourceThatShouldWin := sources.NewLogSource("testSource", &config.LogsConfig{})
	sourceThatShouldLose := sources.NewLogSource("testSource2", &config.LogsConfig{})
	launcher.sources <- sourceThatShouldWin
	launcher.sources <- sourceThatShouldLose

	launcher.stop <- struct{}{}

	assert.Equal(t, 1, len(launcher.tailers))

	assert.Equal(t, "journald:default", sourceThatShouldWin.GetInputs()[0])
	assert.Equal(t, 0, len(sourceThatShouldLose.GetInputs()))
}

func TestMultipleTailersDifferentPath(t *testing.T) {
	launcher := newTestLauncher(t)

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestMultipleTailersOnSamePath(t *testing.T) {
	launcher := newTestLauncher(t)

	sourceThatShouldWin := sources.NewLogSource("testSource", &config.LogsConfig{Path: "/foo/bar"})
	sourceThatShouldLose := sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})
	launcher.sources <- sourceThatShouldWin
	launcher.sources <- sourceThatShouldLose

	launcher.stop <- struct{}{}

	assert.Equal(t, 1, len(launcher.tailers))

	assert.Equal(t, "journald:/foo/bar", sourceThatShouldWin.GetInputs()[0])
	assert.Equal(t, 0, len(sourceThatShouldLose.GetInputs()))
}

func TestMultipleTailersSamePathWithId(t *testing.T) {
	launcher := newTestLauncher(t)

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{Path: "/foo/bar", ConfigID: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar", ConfigID: "bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestMultipleTailersWithId(t *testing.T) {
	launcher := newTestLauncher(t)

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{ConfigID: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{ConfigID: "bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestStopLauncher(t *testing.T) {
	launcher := newTestLauncher(t)

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})

	launcher.Stop()

	assert.Equal(t, 0, len(launcher.tailers))
}
