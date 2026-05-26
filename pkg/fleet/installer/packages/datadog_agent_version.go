// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package packages

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// getCurrentAgentVersion returns the current agent version in URL-safe format with -1 suffix.
//
//nolint:unused // Used in platform-specific files
func getCurrentAgentVersion() string {
	v := version.AgentVersionURLSafe
	if strings.HasSuffix(v, "-1") {
		return v
	}
	return v + "-1"
}
