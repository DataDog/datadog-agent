package agentchecks

import (
	"encoding/json"
	"fmt"
)

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	AgentChecks []interface{} `json:"agent_checks"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	// use an alias to avoid infinit recursion while serializing
	type PayloadAlias Payload

	return json.Marshal((*PayloadAlias)(p))
}

// Marshal not implemented
func (p *Payload) Marshal() ([]byte, error) {
	return nil, fmt.Errorf("V5 Payload serialization is not implemented")
}
