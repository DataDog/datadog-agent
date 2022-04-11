package inferredspan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetSynchronicityFalse(t *testing.T) {
	var attributes EventKeys
	attributes.Headers.InvocationType = ""
	span := GenerateSpan()
	setSynchronicity(&span, attributes)

	assert.False(t, span.IsAsync)
}

func TestSetSynchronicityTrue(t *testing.T) {
	var attributes EventKeys
	attributes.Headers.InvocationType = "Event"
	span := GenerateSpan()
	setSynchronicity(&span, attributes)

	assert.True(t, span.IsAsync)
}
