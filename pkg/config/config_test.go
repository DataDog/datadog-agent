package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaults(t *testing.T) {
	assert.Equal(t, Datadog.GetString("dd_url"), "http://localhost:17123")
}
