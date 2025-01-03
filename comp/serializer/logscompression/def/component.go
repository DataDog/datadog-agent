package logcompressor

import (
	factory "github.com/DataDog/datadog-agent/comp/serializer/compressionfactory/def"
)

type Component interface {
	factory.Component
}
