package collectorContrib

import (
	"go.opentelemetry.io/collector/otelcol"
)

// Component is the interface for the collector-contrib
type Component interface {
	OTelComponentFactories() (otelcol.Factories, error)
}
