// +build linux windows darwin

package v5

import (
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
