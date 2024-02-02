// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package agentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// agentEnabled determines whether the process agent is enabled based on the configuration
// The process-agent always runs as a stand alone agent in all non-linux platforms
func agentEnabled(_ processAgentParams) bool {
	return flavor.GetFlavor() == flavor.ProcessAgent
}
