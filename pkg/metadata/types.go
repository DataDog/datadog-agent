package metadata

import "github.com/DataDog/datadog-agent/pkg/forwarder"

// Provider is anything capable to collect and send metadata payloads
// through the forwarder
type Provider interface {
	Send(apiKey string, fwd forwarder.Forwarder) error
}
