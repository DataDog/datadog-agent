package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/flags/features"
	"github.com/stretchr/testify/assert"
)

func TestWithFeatures(t *testing.T) {
	assert.False(t, features.HasFeature("unknown_feature"))
	undo := WithFeatures("unknown_feature,other")
	assert.True(t, features.HasFeature("unknown_feature"))
	assert.True(t, features.HasFeature("other"))
	undo()
	assert.False(t, features.HasFeature("unknown_feature"))
	assert.False(t, features.HasFeature("other"))
}
