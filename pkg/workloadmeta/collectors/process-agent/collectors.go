package process_agent

import (
	"github.com/DataDog/datadog-agent/pkg/config"

	// register collectors
	processCollector "github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/internal/process"
)

func ProcessCollectorEnabled(cfg config.ConfigReader) bool {
	return processCollector.Enabled(cfg)
}
