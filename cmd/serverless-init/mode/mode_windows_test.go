// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package mode

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunInitUnsupportedOnWindows(t *testing.T) {
	err := RunInit(nil)
	assert.EqualError(t, err, "serverless-init is not supported on windows")
}

func TestRunSidecarUnsupportedOnWindows(t *testing.T) {
	err := RunSidecar(nil)
	assert.EqualError(t, err, "serverless-init is not supported on windows")
}
