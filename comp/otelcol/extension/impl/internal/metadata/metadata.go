package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	Type = component.MustNewType("ddextension")
)

const (
	ExtensionStability = component.StabilityLevelDevelopment
)
