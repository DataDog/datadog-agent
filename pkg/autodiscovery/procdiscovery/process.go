package procdiscovery

import (
	"encoding/json"
	"expvar"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/procmatch"
	"github.com/shirou/gopsutil/process"
)

// IntegrationProcess represents a process that matches an integration
type IntegrationProcess struct {
	Cmd         string `json:"cmd"`          // The command line that matched the integration
	DisplayName string `json:"display_name"` // The integration display name
	Name        string `json:"name"`         // The integration name
	PID         int32  `json:"pid"`          // The PID of the given process
}

// DiscoveredIntegrations is a map whose keys are integrations names and values are lists of IntegrationProcess
type DiscoveredIntegrations struct {
	Discovered map[string][]IntegrationProcess
	Running    map[string]struct{}
	Failing    map[string]struct{}
}

// DiscoverIntegrations retrieves processes running on the host and tries to find possible integrations
func DiscoverIntegrations(discoverOnly bool) (DiscoveredIntegrations, error) {
	di := DiscoveredIntegrations{}
	matcher, err := procmatch.NewDefault()

	if err != nil {
		return di, fmt.Errorf("Couldn't build default process matcher: %s", err)
	}

	processes, err := process.Processes()

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
	processList := map[string]process.Process{}
	for _, p := range processes {
		cmd, err := p.Cmdline()
		if err != nil {
			continue
		}
		processList[cmd] = *p
	}

	integrations := map[string][]IntegrationProcess{}

	// Try to find an integration for each process
	for process := range processList {
		matched := matcher.Match(process)
		if len(matched.Name) == 0 {
			continue
		}

		if _, ok := integrations[matched.Name]; !ok {
			integrations[matched.Name] = []IntegrationProcess{}
		}

		integrations[matched.Name] = append(integrations[matched.Name], IntegrationProcess{
			Cmd:         process,
			DisplayName: matched.DisplayName,
			Name:        matched.Name,
			PID:         processList[process].Pid,
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
