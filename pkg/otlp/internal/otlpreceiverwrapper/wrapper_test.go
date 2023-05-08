package otlpreceiverwrapper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
)

func TestFactoryComponentType(t *testing.T) {
	t.Run("verify otlp type", func(t *testing.T) {
		assert.Equal(t, component.Type("otlp"), NewFactory().Type())
	})
}
