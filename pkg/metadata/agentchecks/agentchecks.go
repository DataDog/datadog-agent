package agentchecks

import (
	"github.com/DataDog/datadog-agent/pkg/collector/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/cihub/seelog"
)

// GetPayload builds a payload of all the agentchecks metadata
func GetPayload() *Payload {
	seelog.Info("I got here!!!!")
	payload := &Payload{
		AgentChecks: []interface{}{},
	}

	checkStats := runner.GetCheckStats()

	for check, stats := range checkStats {
		var status []interface{}
		if stats.LastError != "" {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastError, "ERROR", "",
			}
		} else if len(stats.LastWarnings) != 0 {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastWarnings, "WARNING", "",
			}
		} else {
			status = []interface{}{
				check, "", stats.CheckID, stats.LastWarnings, "OK", "",
			}
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	loaderErrors := autodiscovery.GetLoaderErrors()

	for check, errs := range loaderErrors {
		status := []interface{}{
			check, "", "initialization", "ERROR", errs,
		}
		payload.AgentChecks = append(payload.AgentChecks, status)
	}

	return payload
}
