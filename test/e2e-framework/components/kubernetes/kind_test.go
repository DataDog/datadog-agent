// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKindBinaryName(t *testing.T) {
	assert := assert.New(t)

	tests := []struct {
		kindVersion string
		want        string
	}{
		{"v0.15.0", "kind015"},
		{"v0.17.0", "kind017"},
		{"v0.20.0", "kind020"},
		{"v0.22.0", "kind022"},
		{"v0.26.0", "kind026"},
		{"v0.27.0", "kind027"},
		{"v0.28.0", "kind028"},
		{"v0.31.0", "kind031"},
		{"v0.32.0", "kind032"},
		{"v0.33.0", "kind033"},
		{"v1.0.0", ""},
		{"v0.32.0-rc.0", ""},
		{"v0.32.1", ""},
		{"not-a-version", ""},
	}
	for _, tt := range tests {
		assert.Equal(tt.want, KindBinaryName(tt.kindVersion), "kind version %q", tt.kindVersion)
	}
}
