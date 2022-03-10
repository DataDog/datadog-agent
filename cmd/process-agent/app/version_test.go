package app

import (
	"fmt"
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
	reset := setVersionForTest("1.33.7", "asdf", "1")
	defer reset()

	var s strings.Builder
	err := WriteVersion(&s)
	require.NoError(t, err)

	assert.Equal(t,
		fmt.Sprintf("Agent 1.33.7 - Commit: asdf - Serialization version: 1 - Go version: %s\n", runtime.Version()),
		s.String(),
	)
}
