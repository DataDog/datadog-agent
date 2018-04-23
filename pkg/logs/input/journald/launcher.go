package journald

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
)

// Launcher is available only on systems with libsystemd
type Launcher struct {
	sources []*config.LogSource
}

// New returns a new Launcher.
func New(sources []*config.LogSource, pipelineProvider pipeline.Provider, auditor *auditor.Auditor) *Launcher {
	journaldSources := []*config.LogSource{}
	for _, source := range sources {
		if source.Config.Type == config.JournaldType {
			journaldSources = append(journaldSources, source)
		}
	}
	return &Launcher{
		sources: journaldSources,
	}
}

// Start does nothing
func (l *Launcher) Start() {
	if len(l.sources) > 0 {
		log.Warn("Journald is not supported on your system yet.")
	}
}

// Stop does nothing
func (l *Launcher) Stop() {
	// does nothing
}
