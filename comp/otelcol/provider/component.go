package provider

import "github.com/DataDog/datadog-agent/comp/otelcol/provider/providerimpl"

// team: opentelemetry

// Component is the component type.
type Component interface {
	providerimpl.ExtendedConfigProvider
}
