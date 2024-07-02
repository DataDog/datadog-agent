// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
			path:     "./testdata/node_2/index.js",
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

func TestNodeJSDetector(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		env      []string
		wantMeta ServiceMetadata
		wantOK   bool
	}{
		{
			name:     "empty_args",
			args:     []string{},
			env:      []string{},
			wantMeta: ServiceMetadata{},
			wantOK:   false,
		},
		{
			name:     "package_json_found",
			args:     []string{"./testdata/node_2/index.js"},
			env:      []string{},
			wantMeta: ServiceMetadata{Name: "my-awesome-package"},
			wantOK:   true,
		},
		{
			name:     "package_json_found_pwd",
			args:     []string{"index.js"},
			env:      []string{"PWD=./testdata/node_2"},
			wantMeta: ServiceMetadata{Name: "my-awesome-package"},
			wantOK:   true,
		},
		{
			name:     "package_json_not_found",
			args:     []string{"./testdata/node_1/server.js"},
			env:      []string{},
			wantMeta: ServiceMetadata{Name: "server"},
			wantOK:   true,
		},
		{
			name:     "package_json_not_found_pwd",
			args:     []string{"server.js"},
			env:      []string{"PWD=./testdata/node_1"},
			wantMeta: ServiceMetadata{Name: "server"},
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &nodeDetector{ctx: DetectionContext{
				logger: zap.NewNop(),
				fs:     &RealFs{},
				args:   []string{}, // these are not used here
				envs:   tt.env,
			}}

			meta, ok := d.detect(tt.args)
			assert.Equal(t, tt.wantMeta, meta)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}
