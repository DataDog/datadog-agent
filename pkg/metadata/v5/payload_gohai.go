// +build linux windows darwin

package v5

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/metadata/gohai"
)

// GohaiPayload wraps Payload from the gohai package
type GohaiPayload struct {
	gohai.Payload
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	ResourcesPayload
	// TODO: host-tags
	// TODO: external_host_tags
	GohaiPayload
	// TODO: agent_checks
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
