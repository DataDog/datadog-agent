package remoteagent

// StatusSection is a map of key-value pairs that represent a section of the status data
type StatusSection map[string]string

// StatusData contains the status data for a remote agent
type StatusData struct {
	AgentID       string
	DisplayName   string
	FailureReason string
	MainSection   StatusSection
	NamedSections map[string]StatusSection
}

// FlareData contains the flare data for a remote agent
type FlareData struct {
	AgentID string
	Files   map[string][]byte
}

// RegistrationData contains the registration information for a remote agent
type RegistrationData struct {
	ID          string
	DisplayName string
	APIEndpoint string
	AuthToken   string
}
