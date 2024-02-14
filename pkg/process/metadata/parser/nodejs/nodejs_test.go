// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodejs

import (
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"testing"
)

func TestFindNameFromNearestPackageJSON(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			"should return false when nothing found",
			"./",
			"",
		},
		{
			"should return false when name is empty",
			"./testData/inner/index.js",
			"",
		},
		{
			"should return true when name is found",
			"./testData/index.js",
			"my-awesome-package",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Clean(tt.path))
			assert.NoError(t, err)
			ok, value := FindNameFromNearestPackageJSON(abs)
			assert.Equal(t, len(tt.expected) > 0, ok)
			assert.Equal(t, tt.expected, value)
		})
	}
}
