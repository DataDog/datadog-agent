// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test
// +build test

package config

// team: agent-apm

import (
	corecompcfg "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
)

func setupConfig(deps dependencies, apikey string) (*config.AgentConfig, error) {
	if apikey != "" {
		if mock, ok := deps.Config.(corecompcfg.Mock); ok && !mock.IsSet("api_key") {
			mock.SetWithoutSource("api_key", apikey)
		}
	}

	return setupConfigCommon(deps, apikey)
}
