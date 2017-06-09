package metadata

import "github.com/DataDog/datadog-agent/pkg/forwarder"

// Collector is anything capable to collect and send metadata payloads
// through the forwarder.
// A Metadata Collector normally uses a Metadata Provider to fill the payload.
type Collector interface {
	Send(apiKey string, fwd forwarder.Forwarder) error
}
