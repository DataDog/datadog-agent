// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/settings"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetupInternalProfiling is a common helper to configure runtime settings for internal profiling.
func SetupInternalProfiling(cfg config.Reader, configPrefix string) {
	if v := cfg.GetInt(configPrefix + "internal_profiling.block_profile_rate"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_block_profile_rate", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error setting block profile rate: %v", err)
		}
	}

	if v := cfg.GetInt(configPrefix + "internal_profiling.mutex_profile_fraction"); v > 0 {
		if err := settings.SetRuntimeSetting("runtime_mutex_profile_fraction", v, model.SourceAgentRuntime); err != nil {
			log.Errorf("Error mutex profile fraction: %v", err)
		}
	}

	if cfg.GetBool(configPrefix + "internal_profiling.enabled") {
		err := settings.SetRuntimeSetting("internal_profiling", true, model.SourceAgentRuntime)
		if err != nil {
			log.Errorf("Error starting profiler: %v", err)
		}
	}
}
