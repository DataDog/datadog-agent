package procdiscovery

import (
	"encoding/json"
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/procmatch"
)

// DiscoverIntegrations retrieves processes running on the host and tries to find possible integrations
func DiscoverIntegrations(discoverOnly bool) (DiscoveredIntegrations, error) {
	di := DiscoveredIntegrations{}
	matcher, err := procmatch.NewDefault()

	if err != nil {
		return di, fmt.Errorf("Couldn't build default process matcher: %s", err)
	}

	processes, err := pollProcesses()

	if err != nil {
		return di, fmt.Errorf("Couldn't retrieve process list: %s", err)
	}

	if !discoverOnly {
		running, failing, err := retrieveIntegrations()
		if err != nil {
			return di, err
		}
		di.Running = running
		di.Failing = failing
	}

	// processList is a set of processes (removes duplicate processes)
	processList := map[string]process{}
	for _, p := range processes {
		processList[p.cmd] = p
	}

	integrations := map[string][]IntegrationProcess{}

	// Try to find an integration for each process
	for proc := range processList {
		matched := matcher.Match(proc)
		if len(matched.Name) == 0 {
			continue
		}

		if _, ok := integrations[matched.Name]; !ok {
			integrations[matched.Name] = []IntegrationProcess{}
		}

		integrations[matched.Name] = append(integrations[matched.Name], IntegrationProcess{
			Cmd:         proc,
			DisplayName: matched.DisplayName,
			Name:        matched.Name,
			PID:         processList[proc].pid,
		})
	}
	di.Discovered = integrations

	return di, nil
}

func retrieveIntegrations() (map[string]struct{}, map[string]struct{}, error) {
	running := map[string]struct{}{}
	failing := map[string]struct{}{}

	st, err := getStatus()

	if err != nil {
		return running, failing, fmt.Errorf("Couldn't retrieve agent status: %s", err)
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
