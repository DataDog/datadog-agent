//go:build systemd
// +build systemd

package journald

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	tailer "github.com/DataDog/datadog-agent/pkg/logs/internal/tailers/journald"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/coreos/go-systemd/sdjournal"
	"gotest.tools/assert"
)

type MockJournal struct {
}

func (m *MockJournal) AddMatch(match string) error                { return nil }
func (m *MockJournal) AddDisjunction() error                      { return nil }
func (m *MockJournal) SeekTail() error                            { return nil }
func (m *MockJournal) SeekHead() error                            { return nil }
func (m *MockJournal) Wait(timeout time.Duration) int             { return 0 }
func (m *MockJournal) SeekCursor(cursor string) error             { return nil }
func (m *MockJournal) NextSkip(skip uint64) (uint64, error)       { return 0, nil }
func (m *MockJournal) Close() error                               { return nil }
func (m *MockJournal) Next() (uint64, error)                      { return 0, nil }
func (m *MockJournal) GetEntry() (*sdjournal.JournalEntry, error) { return nil, sdjournal.ErrExpired }
func (m *MockJournal) GetCursor() (string, error)                 { return "", nil }

func newTestLauncher() *Launcher {
	return &Launcher{
		sources:              make(chan *sources.LogSource),
		pipelineProvider:     pipeline.NewMockProvider(),
		registry:             auditor.New("", "registry.json", time.Hour, health.RegisterLiveness("fake")),
		tailers:              make(map[string]*tailer.Tailer),
		stop:                 make(chan struct{}),
		newJournalFn:         func() (tailer.Journal, error) { return &MockJournal{}, nil },
		newJournalFromPathFn: func(path string) (tailer.Journal, error) { return &MockJournal{}, nil },
	}
}

func TestSingeJournaldConfig(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{})

	launcher.stop <- struct{}{}

	assert.Equal(t, 1, len(launcher.tailers))
}

func TestMultipleTailersDifferentPath(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestMultipleTailersSamePathOverrridesFirst(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{Path: "/foo/bar"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 1, len(launcher.tailers))
}

func TestMultipleTailersSamePathWithId(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{Path: "/foo/bar", ConfigId: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar", ConfigId: "bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestMultipleTailersWithId(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{ConfigId: "foo"})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{ConfigId: "bar"})

	launcher.stop <- struct{}{}

	assert.Equal(t, 2, len(launcher.tailers))
}

func TestStopLauncher(t *testing.T) {
	launcher := newTestLauncher()
	go launcher.run()

	launcher.sources <- sources.NewLogSource("testSource", &config.LogsConfig{})
	launcher.sources <- sources.NewLogSource("testSource2", &config.LogsConfig{Path: "/foo/bar"})

	launcher.Stop()

	assert.Equal(t, 0, len(launcher.tailers))
}
