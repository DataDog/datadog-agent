// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTailerNextLogSinceDate(t *testing.T) {
	tailer := &Tailer{}
	assert.Equal(t, "2008-01-12T01:01:01.000000001Z", tailer.nextLogSinceDate("2008-01-12T01:01:01.000000000Z"))
	assert.Equal(t, "2008-01-12T01:01:01.anything", tailer.nextLogSinceDate("2008-01-12T01:01:01.anything"))
	assert.Equal(t, "", tailer.nextLogSinceDate(""))
}

func TestTailerIdentifier(t *testing.T) {
	tailer := &Tailer{ContainerID: "test"}
	assert.Equal(t, "docker:test", tailer.Identifier())
}
