// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsermodAccountData(t *testing.T) {
	for _, shadow := range []bool{false, true} {
		original := "# retained\nother:x:123:dd-agent,alice\ndocker:x:998:alice"
		if shadow {
			original = "# retained\nother:!:admin:dd-agent,alice\ndocker:!hash:admin:alice"
		}
		updated, err := appendDockerMember([]byte(original), shadow)
		require.NoError(t, err)
		require.Equal(t, original+",dd-agent", string(updated))
		again, err := appendDockerMember(updated, shadow)
		require.NoError(t, err)
		require.Equal(t, updated, again)
	}
	for _, input := range []string{"", "other:x:1:\n", "docker:x:0:\n", "docker:x:nope:\n", "docker:x:998:a,,b\n", "docker:x:998:\ndocker:x:999:\n", "docker:x:998:\r\n", "docker:x:998:\x00", "docker:x:998:" + strings.Repeat("a", 64<<10)} {
		_, err := appendDockerMember([]byte(input), false)
		require.Error(t, err, "%q", input[:min(len(input), 40)])
	}
	require.NoError(t, validateDDAgent([]byte("dd-agent:x:123:123::/var/lib/dd-agent:/bin/false\n")))
	for _, input := range []string{"root:x:0:0::/:/bin/sh\n", "dd-agent:x:0:123::/:/bin/sh\n", "dd-agent:x:123:123::/:/bin/sh\ndd-agent:x:124:124::/:/bin/sh\n", "dd-agent:x:123:123"} {
		require.Error(t, validateDDAgent([]byte(input)))
	}
}
