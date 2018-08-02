package collector

import (
	"encoding/json"
	"expvar"
)

// GetRunnerStatus returns status info for the check runner.
func GetRunnerStatus() map[string]interface{} {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	return runnerStats
}

// GetCheckSchedulerStatus returns status info for the check scheduler.
func GetCheckSchedulerStatus() map[string]interface{} {
	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	checkSchedulerStats := make(map[string]interface{})
	json.Unmarshal(checkSchedulerStatsJSON, &checkSchedulerStats)
	return checkSchedulerStats
}
