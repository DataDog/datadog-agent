package logcompressor

import (
	compression "github.com/DataDog/datadog-agent/comp/serializer/compression/factory/def"
)

type Component interface {
	compression.Component
}
