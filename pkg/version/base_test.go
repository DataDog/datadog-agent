// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAgentVersion(t *testing.T) {
	assert.NotEmpty(t, AgentVersion)
}

func TestCommit(t *testing.T) {
	assert.NotEmpty(t, Commit, "The Commit var is empty, indicating that the package was not built correctly")
}
