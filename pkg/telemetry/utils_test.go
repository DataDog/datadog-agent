package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/StackVista/stackstate-agent/pkg/config"
)

func TestIsCheckEnabled(t *testing.T) {
	assert := assert.New(t)

	mockConfig := config.Mock()
	mockConfig.Set("telemetry.enabled", false)

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"*"})

	assert.True(IsCheckEnabled("cpu"))
	assert.True(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"cpu"})

	assert.True(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", false)
	mockConfig.Set("telemetry.checks", []string{"cpu"})

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"cpu", "disk"})

	assert.True(IsCheckEnabled("cpu"))
	assert.True(IsCheckEnabled("disk"))
}
