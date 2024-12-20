package metriccompressor

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/factory/def"
)

// Component is the component type.
type Component interface {
	compression.Compressor
}
