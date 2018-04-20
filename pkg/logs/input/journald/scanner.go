package journald

import (
	"strings"

	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
)

// Scanner is in charge of starting and stopping new journald tailers
type Scanner struct {
	sources          []*config.LogSource
	pipelineProvider pipeline.Provider
	auditor          *auditor.Auditor
	tailers          map[string]*Tailer
}

// New returns a new Scanner.
func New(sources []*config.LogSource, pipelineProvider pipeline.Provider, auditor *auditor.Auditor) *Scanner {
	journaldSources := []*config.LogSource{}
	for _, source := range sources {
		if source.Config.Type == config.JournaldType {
			journaldSources = append(journaldSources, source)
		}
	}
	return &Scanner{
		sources:          journaldSources,
		pipelineProvider: pipelineProvider,
		auditor:          auditor,
		tailers:          make(map[string]*Tailer),
	}
}

// Start starts new tailers.
func (s *Scanner) Start() {
	for _, source := range s.sources {
		identifier := source.Config.Path
		if _, exists := s.tailers[identifier]; exists {
			// set up only one tailer per journal
			continue
		}
		tailer, err := s.setupTailer(source)
		if err != nil {
			log.Warn("Could not set up journald tailer: ", err)
		} else {
			s.tailers[identifier] = tailer
		}
	}
}

// Stop stops all active tailers
func (s *Scanner) Stop() {
	stopper := restart.NewParallelStopper()
	for _, tailer := range s.tailers {
		stopper.Add(tailer)
		delete(s.tailers, tailer.Identifier())
	}
	stopper.Stop()
}

// setupTailer configures and starts a new tailer,
// returns the tailer or an error.
func (s *Scanner) setupTailer(source *config.LogSource) (*Tailer, error) {
	var units []string
	if source.Config.Unit != "" {
		units = strings.Split(source.Config.Unit, ",")
	}
	config := JournalConfig{
		Units: units,
		Path:  source.Config.Path,
	}
	tailer := NewTailer(config, source, s.pipelineProvider.NextPipelineChan())
	err := tailer.Start(s.auditor.GetLastCommittedCursor(tailer.Identifier()))
	if err != nil {
		return nil, err
	}
	return tailer, nil
}
