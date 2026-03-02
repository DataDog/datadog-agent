// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kernel

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestArch(t *testing.T) {
	arch := Arch()
	assert.NotEmpty(t, arch, "Arch() should return a non-empty string")

	// Verify the returned architecture is valid based on current GOARCH
	switch runtime.GOARCH {
	case "386", "amd64":
		assert.Equal(t, "x86", arch)
	case "arm":
		assert.Equal(t, "arm", arch)
	case "arm64":
		assert.Equal(t, "arm64", arch)
	case "ppc64", "ppc64le":
		assert.Equal(t, "powerpc", arch)
	case "mips", "mipsle", "mips64", "mips64le":
		assert.Equal(t, "mips", arch)
	case "riscv64":
		assert.Equal(t, "riscv", arch)
	case "s390x":
		assert.Equal(t, "s390", arch)
	default:
		// For unknown architectures, should return runtime.GOARCH
		assert.Equal(t, runtime.GOARCH, arch)
	}
}
