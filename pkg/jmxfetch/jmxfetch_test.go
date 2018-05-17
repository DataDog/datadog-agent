// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build jmx

package jmxfetch

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultValues(t *testing.T) {
	expected := New()
	j := &JMXFetch{}

	j.setDefaults()

	assert.Equal(t, expected, j)
}
