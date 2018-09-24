package procdiscovery

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
	Error      string `json:"error"`
}

// Checks represents the runnning and failing checks
type Checks struct {
	Running map[string]struct{}
	Failing map[string]struct{}
}

type process struct {
	pid int32
	cmd string
}
