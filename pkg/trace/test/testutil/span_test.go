package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/stretchr/testify/assert"
)

func TestRandomSpan(t *testing.T) {
	assert := assert.New(t)

	for i := 0; i < 1000; i++ {
		s := RandomSpan()
		err := agent.Normalize(s)
		assert.Nil(err)
	}
}

func TestTestSpan(t *testing.T) {
	assert := assert.New(t)
	ts := TestSpan()
	err := agent.Normalize(ts)
	assert.Nil(err)
}
