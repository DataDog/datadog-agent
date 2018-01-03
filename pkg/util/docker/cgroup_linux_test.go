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

// TestDindContainer is to test if our agent can handle dind container correctly
func TestDindContainer(t *testing.T) {
	containerID := "6ab998413f7ae63bb26403dfe9e7ec02aa92b5cfc019de79da925594786c985f"
	tempFolder, cgroup, err := newDindContainerCgroup("dind-container", "memory", containerID)
	assert.NoError(t, err)
	tempFolder.add("memory.limit_in_bytes", "1234")
	defer tempFolder.removeAll()

	value, err := cgroup.MemLimit()
	assert.NoError(t, err)
	assert.Equal(t, value, uint64(1234))
}
