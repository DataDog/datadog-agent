package v5

import (
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/collector/metadata/host"
)

// CommonPayload wraps Payload from the common package
type CommonPayload struct {
	common.Payload
}

// HostPayload wraps Payload from the host package
type HostPayload struct {
	host.Payload
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	CommonPayload
	HostPayload
	// TODO: resources
	// TODO: host-tags
	// TODO: external_host_tags
	// TODO: gohai
	// TODO: agent_checks
}
