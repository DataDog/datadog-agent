// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !linux

package agent

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	logComponent "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// Enabled determines whether the process agent is enabled based on the configuration
// The process-agent always runs as a stand-alone agent in all non-linux platforms
func Enabled(_ config.Component, _ []types.CheckComponent, _ logComponent.Component) bool {
	return flavor.GetFlavor() == flavor.ProcessAgent
}
