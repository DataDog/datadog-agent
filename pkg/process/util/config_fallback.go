// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package util

import "github.com/DataDog/datadog-agent/pkg/config/model"

// GetRunInCoreAgentConfig returns false in place of the config value for process_config.run_in_core_agent.enabled.
func GetRunInCoreAgentConfig(_ model.Reader) bool {
	return false
}
