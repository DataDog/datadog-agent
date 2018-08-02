package collector

import (
	"encoding/json"
	"expvar"
)

func GetRunnerStatus() map[string]interface{} {
	runnerStatsJSON := []byte(expvar.Get("runner").String())
	runnerStats := make(map[string]interface{})
	json.Unmarshal(runnerStatsJSON, &runnerStats)
	return runnerStats
}

func GetCheckSchedulerStatus() map[string]interface{} {
	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	checkSchedulerStats := make(map[string]interface{})
	json.Unmarshal(checkSchedulerStatsJSON, &checkSchedulerStats)
	return checkSchedulerStats
}
