package metadata

import (
	"go.opentelemetry.io/collector/component"
)

var (
	Type = component.MustNewType("datadog")
)

const (
	ExtensionStability = component.StabilityLevelDevelopment
)
