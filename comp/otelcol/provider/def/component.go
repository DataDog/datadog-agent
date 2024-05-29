package provider

import (
	"go.opentelemetry.io/collector/otelcol"
)

// team: opentelemetry

// Component is the component type.
type Component interface {
	ExtendedConfigProvider
}

// ExtendedConfigProvider implements the otelcol.ConfigProvider interface and
// provides extra functions to expose the provided and enhanced configs.
type ExtendedConfigProvider interface {
	otelcol.ConfigProvider
	GetProvidedConf() string
	GetEnhancedConf() string
}
