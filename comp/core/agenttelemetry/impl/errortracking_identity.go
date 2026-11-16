// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agenttelemetryimpl

import (
	"slices"
	"strings"
	"sync"

	agentversion "github.com/DataDog/datadog-agent/pkg/version"
)

// Identity tags appended to every errortracking event. The names are part of
// the contract with Error Tracking and monitors, so keep them stable:
//
//   - agent.version: version without build metadata, identical to the
//     DD-Telemetry-Product-Version header (for example 7.85.0-rc.2), so events
//     from releases that predate these tags can still be grouped by the header.
//   - agent.rc: "true" when the pre-release part has an "rc" segment
//     (7.85.0-rc.2), "false" otherwise (releases and development builds).
const (
	agentVersionTag = "agent.version"
	agentRCTag      = "agent.rc"
)

// identityTags returns the identity tags for the running binary as a suffix
// to append to an existing comma-separated tags string: empty, or starting
// with a comma. It is computed once because the version is fixed at build time.
var identityTags = sync.OnceValue(func() string { return buildIdentityTags(agentversion.AgentVersion) })

func buildIdentityTags(agentVersion string) string {
	v, err := agentversion.New(agentVersion, "")
	if err != nil {
		return ""
	}
	rc := "false"
	if slices.Contains(strings.Split(v.Pre, "."), "rc") {
		rc = "true"
	}
	return "," + agentVersionTag + ":" + v.GetNumberAndPre() + "," + agentRCTag + ":" + rc
}
