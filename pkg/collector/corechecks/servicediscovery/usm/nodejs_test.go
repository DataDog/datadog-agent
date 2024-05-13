package usm

import (
	"testing"

	"go.uber.org/zap"

	"github.com/stretchr/testify/assert"
)

func TestFindNameFromNearestPackageJSON(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "should return false when nothing found",
			path:     "./",
			expected: "",
		},
		{
			name:     "should return false when name is empty",
			path:     "./testdata/inner/index.js",
			expected: "",
		},
		{
			name:     "should return true when name is found",
			path:     "./testdata/index.js",
			expected: "my-awesome-package",
		},
	}
	instance := &nodeDetector{ctx: DetectionContext{logger: zap.NewNop(), fs: &RealFs{}}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, ok := instance.findNameFromNearestPackageJSON(tt.path)
			assert.Equal(t, len(tt.expected) > 0, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}
