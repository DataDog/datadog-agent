package testutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/config/features"
	"github.com/stretchr/testify/assert"
)

func TestWithFeatures(t *testing.T) {
	assert.False(t, features.Has("unknown_feature"))
	undo := WithFeatures("unknown_feature,other")
	assert.True(t, features.Has("unknown_feature"))
	assert.True(t, features.Has("other"))
	undo()
	assert.False(t, features.Has("unknown_feature"))
	assert.False(t, features.Has("other"))
}
