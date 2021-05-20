package testutil

import (
	"testing"

	featuresConfig "github.com/DataDog/datadog-agent/pkg/trace/export/config/features"
	"github.com/stretchr/testify/assert"
)

func TestWithFeatures(t *testing.T) {
	assert.False(t, featuresConfig.HasFeature("unknown_feature"))
	undo := WithFeatures("unknown_feature,other")
	assert.True(t, featuresConfig.HasFeature("unknown_feature"))
	assert.True(t, featuresConfig.HasFeature("other"))
	undo()
	assert.False(t, featuresConfig.HasFeature("unknown_feature"))
	assert.False(t, featuresConfig.HasFeature("other"))
}
