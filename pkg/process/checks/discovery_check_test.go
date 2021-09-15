package checks

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
)

func TestProcessDiscoveryCheck(t *testing.T) {
	cfg := &config.AgentConfig{MaxPerMessage: 10}
	ProcessDiscovery.Init(cfg, &model.SystemInfo{
		Cpus:        []*model.CPUInfo{{Number: 0}},
		TotalMemory: 0,
	})

	// Test check runs without error
	result, err := ProcessDiscovery.Run(cfg, 0)
	if err != nil {
		t.Error(err)
	}

	// Test that result has the proper number of chunks, and that those chunks are of the correct type
	for _, elem := range result {
		collectorProcDiscovery, ok := elem.(*model.CollectorProcDiscovery)
		if !ok {
			t.Error("Expected CollectorProcDiscovery type from ProcessDiscovery check payload, got something else.")
		}
		if len(collectorProcDiscovery.ProcessDiscoveries) > cfg.MaxPerMessage {
			t.Errorf("Expected less than %d messages in chunk, got %d",
				cfg.MaxPerMessage, len(collectorProcDiscovery.ProcessDiscoveries))
		}
	}
}
