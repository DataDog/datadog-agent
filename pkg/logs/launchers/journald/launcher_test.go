// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build systemd

package journald

import (
	"testing"
	"time"

	"github.com/coreos/go-systemd/sdjournal"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/flare"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

type MockJournal struct{}

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) AddMatch(match string) error { return nil }
func (m *MockJournal) AddDisjunction() error       { return nil }
func (m *MockJournal) SeekTail() error             { return nil }
func (m *MockJournal) SeekHead() error             { return nil }

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) Wait(timeout time.Duration) int { return 0 }

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) SeekCursor(cursor string) error { return nil }

//nolint:revive // TODO(AML) Fix revive linter
func (m *MockJournal) NextSkip(skip uint64) (uint64, error)       { return 0, nil }
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

//nolint:revive // TODO(AML) Fix revive linter
func (s *MockJournalFactory) NewJournalFromPath(path string) (tailer.Journal, error) {
	return &MockJournal{}, nil
}

func newTestLauncher(t *testing.T) *Launcher {
	t.Helper()

	fakeTagger := taggerimpl.SetupFakeTagger(t)

	launcher := NewLauncherWithFactory(&MockJournalFactory{}, flare.NewFlareController(), fakeTagger)
	launcher.Start(launchers.NewMockSourceProvider(), pipeline.NewMockProvider(), auditor.New("", "registry.json", time.Hour, health.RegisterLiveness("fake")), tailers.NewTailerTracker())
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

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{Path: "/foo/bar", ConfigId: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar", ConfigId: "bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestMultipleTailersWithId(t *testing.T) {
	launcher := newTestLauncher(t)

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{ConfigId: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{ConfigId: "bar"})

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
