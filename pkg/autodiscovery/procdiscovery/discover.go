package procdiscovery

import (
	"encoding/json"
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/procmatch"
)

// DiscoverIntegrations retrieves processes running on the host and tries to find possible integrations
func DiscoverIntegrations() (map[string][]IntegrationProcess, error) {
	matcher, err := procmatch.NewDefault()

	if err != nil {
		return nil, fmt.Errorf("Couldn't build default process matcher: %s", err)
	}

	processes, err := pollProcesses()

	if err != nil {
		return nil, fmt.Errorf("Couldn't retrieve process list: %s", err)
	}

	integrations := map[string][]IntegrationProcess{}

	// Try to find an integration for each process
	for _, proc := range processes {
		matched := matcher.Match(proc.cmd)
		if len(matched.Name) == 0 {
			continue
		}

		if _, ok := integrations[matched.Name]; !ok {
			integrations[matched.Name] = []IntegrationProcess{}
		}

		integrations[matched.Name] = append(integrations[matched.Name], IntegrationProcess{
			Cmd:         proc.cmd,
			DisplayName: matched.DisplayName,
			Name:        matched.Name,
			PID:         proc.pid,
		})
	}

	return integrations, nil
}

// GetChecks retrieves the running and failing checks
func GetChecks() (Checks, error) {
	ru, fa, err := retrieveIntegrations()
	if err != nil {
		return Checks{}, err
	}

	return Checks{Running: ru, Failing: fa}, nil
}

func retrieveIntegrations() (map[string]struct{}, map[string]struct{}, error) {
	running := map[string]struct{}{}
	failing := map[string]struct{}{}

	st, err := getStatus()

	if err != nil {
		return running, failing, fmt.Errorf("couldn't retrieve agent status: %s", err)
	}
	for key := range st.RunnerStats.Checks {
		running[key] = struct{}{}
	}
	for key := range st.CheckSchedulerStats.LoaderErrors {
		failing[key] = struct{}{}
	}

	return running, failing, nil
}

type status struct {
	RunnerStats struct {
		Checks map[string]interface{}
	} `json:"runnerStats"`
	CheckSchedulerStats struct {
		LoaderErrors map[string]interface{}
	} `json:"checkSchedulerStats"`
}

func getStatus() (status, error) {
	stats := status{}

	runnerStatsJSON := []byte(expvar.Get("runner").String())
	err := json.Unmarshal(runnerStatsJSON, &stats.RunnerStats)
	if err != nil {
		return stats, fmt.Errorf("An error occurred unmarshalling runner stats: %s", err)
	}

	checkSchedulerStatsJSON := []byte(expvar.Get("CheckScheduler").String())
	err = json.Unmarshal(checkSchedulerStatsJSON, &stats.CheckSchedulerStats)
	if err != nil {
		return stats, fmt.Errorf("An error occurred unmarshalling check scheduler stats: %s", err)
	}

	return stats, nil
}
