// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStaleLinksTelemetry(t *testing.T) {
	count := func() float64 {
		return staleLinksTelemetry.WithValues(withPlatformTags()...).Get()
	}
	before := count()

	countStaleLinks(0)
	assert.Equal(t, before, count())

	countStaleLinks(2)
	countStaleLinks(1)
	assert.Equal(t, before+3, count())
}
