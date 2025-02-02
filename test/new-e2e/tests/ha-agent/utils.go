package haagent

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  *haAgentMetadata `json:"ha_agent_metadata"`
}

type haAgentMetadata struct {
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
}
