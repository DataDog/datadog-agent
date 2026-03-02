// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRuntimeArch(t *testing.T) {
	result := RuntimeArch()

	switch runtime.GOARCH {
	case "amd64":
		assert.Equal(t, "x64", result)
	case "arm64":
		assert.Equal(t, "arm64", result)
	default:
		assert.Equal(t, runtime.GOARCH, result)
	}
}
