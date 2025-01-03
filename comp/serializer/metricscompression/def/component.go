package metriccompressor

import (
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// Component is the component type.
type Component interface {
	compression.Compressor
}
