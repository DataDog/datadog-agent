// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComputeClientAPIVersion(t *testing.T) {
	var version string
	var err error

	// should raise an error
	_, err = computeClientAPIVersion("1.12")
	assert.NotNil(t, err)

	// should return a version lower than the max
	version, err = computeClientAPIVersion("1.24")
	assert.Nil(t, err)
	assert.Equal(t, "1.24", version)

	// should return the max version
	version, err = computeClientAPIVersion("1.35")
	assert.Nil(t, err)
	assert.Equal(t, maxVersion, version)
}
