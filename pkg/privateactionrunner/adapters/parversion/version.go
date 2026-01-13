// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package parversion provides an adapter that bridges to the original source code
package parversion

import (
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var RunnerVersion string

func init() {
	agentVersion, err := version.Agent()
	if err != nil {
		log.Error("Failed to get agent version", log.ErrorField(err))
		RunnerVersion = version.AgentVersion
	} else {
		RunnerVersion = agentVersion.String()
	}
}
