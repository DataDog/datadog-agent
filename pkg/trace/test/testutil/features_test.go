package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/stretchr/testify/assert"
)

func TestWithFeatures(t *testing.T) {
	assert.False(t, config.HasFeature("unknown_feature"))
	undo := WithFeatures("unknown_feature,other")
	assert.True(t, config.HasFeature("unknown_feature"))
	assert.True(t, config.HasFeature("other"))
	undo()
	assert.False(t, config.HasFeature("unknown_feature"))
	assert.False(t, config.HasFeature("other"))
}
