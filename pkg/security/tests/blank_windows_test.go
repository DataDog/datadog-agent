// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && functionaltests

// Package tests holds tests related files
package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThatWindowsCanRunATest(t *testing.T) {
	assert.Equal(t, 2, 2)
}
