// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux || windows

package dockerpermissions

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The id must be deterministic (stable across ticks) and keep the kebab-case prefix.
func TestSocketSetIssueID_StableAndPrefixed(t *testing.T) {
	base := socketSetIssueID("host-a", []string{"/var/run/docker.sock"})

	assert.Equal(t, base, socketSetIssueID("host-a", []string{"/var/run/docker.sock"}), "id must be stable for the same host and socket set")
	assert.True(t, strings.HasPrefix(base, IssueID+":"), "id %q must keep the %q prefix", base, IssueID)
}

// Host and socket set must both scope the id so no two distinct instances collapse.
func TestSocketSetIssueID_ScopedToHostAndSockets(t *testing.T) {
	base := socketSetIssueID("host-a", []string{"/var/run/docker.sock"})

	assert.NotEqual(t, base, socketSetIssueID("host-b", []string{"/var/run/docker.sock"}), "hostname must scope the id")
	assert.NotEqual(t, base, socketSetIssueID("host-a", []string{"//./pipe/docker_engine"}), "a different socket must change the id")
	assert.NotEqual(t, base, socketSetIssueID("host-a", []string{"/host/var/run/docker.sock", "/var/run/docker.sock"}), "adding a socket must change the id")
}

// The delimiter keeps concatenation-ambiguous sets distinct: {"a","bc"} != {"ab","c"}.
func TestSocketSetIssueID_DelimiterDisambiguates(t *testing.T) {
	assert.NotEqual(t,
		socketSetIssueID("host-a", []string{"a", "bc"}),
		socketSetIssueID("host-a", []string{"ab", "c"}),
	)
}
