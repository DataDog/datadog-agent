package common

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	APIKey       string `json:"apiKey"`
	AgentVersion string `json:"agentVersion"`
}
