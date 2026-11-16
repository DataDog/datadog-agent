// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log/errortracking"
)

// errorLogTags builds the comma-separated tags string of an error log.
//
// Go-origin records carry the agent repository and commit so Source Code
// Integration and ownership lookups resolve the Go frames. Records whose stack
// was captured outside the Go runtime (Integration is set) hold frames that
// do not exist in this repository, so the git tags are omitted and the
// integration is tagged instead. The commit SHA is a placeholder replaced
// after scrubbing; see commitSHAPlaceholder.
func errorLogTags(e errortracking.ErrorLog) string {
	if e.Integration != "" {
		return "integration:" + e.Integration + ",agent.flavor:" + flavor.GetFlavor()
	}
	return "git.repository_url:https://github.com/DataDog/datadog-agent,git.commit.sha:" + commitSHAPlaceholder + ",agent.flavor:" + flavor.GetFlavor()
}
