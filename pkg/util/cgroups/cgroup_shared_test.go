// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCPUSetParsing(t *testing.T) {
	assert.EqualValues(t, ParseCPUSetFormat("0,1,5-8"), 6)
	assert.EqualValues(t, ParseCPUSetFormat("1"), 1)
	assert.EqualValues(t, ParseCPUSetFormat("2-3"), 2)
}
