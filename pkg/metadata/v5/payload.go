package v5

import (
	"github.com/DataDog/datadog-agent/pkg/metadata/common"
	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/metadata/resources"
)

// CommonPayload wraps Payload from the common package
type CommonPayload struct {
	common.Payload
}

// HostPayload wraps Payload from the host package
type HostPayload struct {
	host.Payload
}

// ResourcesPayload wraps Payload from the resources package
type ResourcesPayload struct {
	resources.Payload
}
