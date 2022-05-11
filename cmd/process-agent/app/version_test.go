// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package app

import (
	"fmt"
	"github.com/fatih/color"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func setVersionForTest(agentVersion, commit, agentPayloadVersion string) (reset func()) {
	oldAgentVersion, oldCommit, oldPayloadVersion := version.AgentVersion, version.Commit, serializer.AgentPayloadVersion
	version.AgentVersion, version.Commit, serializer.AgentPayloadVersion = agentVersion, commit, agentPayloadVersion
	return func() {
		version.AgentVersion, version.Commit, serializer.AgentPayloadVersion = oldAgentVersion, oldCommit, oldPayloadVersion
	}
}

func TestVersion(t *testing.T) {
	reset := setVersionForTest("1.33.7+yeet", "asdf", "1")
	defer reset()

	var s strings.Builder
	err := WriteVersion(&s)
	require.NoError(t, err)

	assert.Equal(t,
		fmt.Sprintf(
			"Agent %s - Meta: %s - Commit: %s - Serialization version: %s - Go version: %s\n",
			color.CyanString("1.33.7"),
			color.YellowString("yeet"),
			color.GreenString("asdf"),
			color.YellowString("1"),
			color.RedString(runtime.Version()),
		),
		s.String(),
	)
}
