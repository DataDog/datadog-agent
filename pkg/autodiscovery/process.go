package autodiscovery

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/procmatch"
	"github.com/shirou/gopsutil/process"
)

// IntegrationProcess represents a process that matches an integration
type IntegrationProcess struct {
	Cmd         string `json:"cmd"`          // The command line that matched the integration
	DisplayName string `json:"display_name"` // The integration display name
	Name        string `json:"name"`         // The integration name
}

// DiscoveredIntegrations is a map whose keys are integrations names and values are lists of IntegrationProcess
type DiscoveredIntegrations map[string][]IntegrationProcess

// DiscoverIntegrations retrieves processes running on the host and tries to find possible integrations
func DiscoverIntegrations() (DiscoveredIntegrations, error) {
	matcher, err := procmatch.NewDefault()

	if err != nil {
		return nil, fmt.Errorf("Couldn't build default process matcher: %s", err)
	}

	processes, err := process.Processes()

	if err != nil {
		return nil, fmt.Errorf("Couldn't retrieve process list: %s", err)
	}

	// processList is a set of processes (removes duplicate processes)
	processList := map[string]struct{}{}
	for _, p := range processes {
		cmd, err := p.Cmdline()
		if err != nil {
			continue
		}
		processList[cmd] = struct{}{}
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
		})
	}

	return integrations, nil
}
