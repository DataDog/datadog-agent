// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

func TestEnrichErrorLog_ExternalStack(t *testing.T) {
	in := errortracking.ErrorLog{
		Time:        time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC),
		Count:       2,
		ErrorKind:   "ConnectionError",
		Stack:       "datadog_checks.redisdb.redisdb.check\n\tdatadog_checks/redisdb/redisdb.py:10 +0x0",
		Integration: "redisdb",
	}

	out := enrichErrorLog(in)

	assert.Equal(t, in.Stack, out.StackTrace, "pre-formatted stack must be shipped as is")
	assert.Equal(t, "ConnectionError", out.ErrorKind)
	assert.Equal(t, 2, out.Count)
	assert.Empty(t, out.Message)
	assert.Contains(t, out.Tags, "integration:redisdb")
	assert.Contains(t, out.Tags, "agent.flavor:"+flavor.GetFlavor())
	assert.NotContains(t, out.Tags, "git.repository_url", "python frames do not live in the agent repository")
	assert.NotContains(t, out.Tags, "git.commit.sha")
}

func TestErrorLogTags_GoOriginKeepsGitTags(t *testing.T) {
	tags := errorLogTags(errortracking.ErrorLog{})

	assert.Contains(t, tags, "git.repository_url:https://github.com/DataDog/datadog-agent")
	assert.Contains(t, tags, "git.commit.sha:"+commitSHAPlaceholder)
	assert.NotContains(t, tags, "integration:")
}
