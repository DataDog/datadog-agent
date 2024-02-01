// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package agentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// agentEnabled determines whether the process agent is enabled based on the configuration
// The process-agent is enabled when the flavor is set to ProcessAgent in all non-linux platforms
func agentEnabled(_ processAgentParams) bool {
	return flavor.GetFlavor() == flavor.ProcessAgent
}
