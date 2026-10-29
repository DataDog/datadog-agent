// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package run

import (
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const coreAgentIPCKey = "system_probe_config.core_agent_ipc.enabled"

// Mirrors the system_probe_config.core_agent_ipc.enabled schema default, used when the
// configuration cannot be read at all.
const defaultCoreAgentIPCEnabled = true

// coreAgentIPCEnabled reports whether system-probe may talk to the core agent. It decides
// which modules go into the fx graph, so it runs before that graph (and the config
// components) are built: the config stream consumer in particular is a dependency of the
// core config component. It loads the configuration with the regular system-probe loader
// into a standalone config object, so the setting resolves exactly as it will later on:
// nested or dotted keys, env vars and fleet policy precedence included.
func coreAgentIPCEnabled(sysprobeConfPath, fleetPoliciesDirPath string) bool {
	cfg, err := sysconfig.NewStandalone(sysprobeConfPath, fleetPoliciesDirPath)
	if err != nil {
		// The regular loader hits the same error right after and fails the startup,
		// so the value returned here does not matter much.
		log.Warnf("unable to read %s early, assuming %t: %v", coreAgentIPCKey, defaultCoreAgentIPCEnabled, err)
		return defaultCoreAgentIPCEnabled
	}
	return cfg.GetBool(coreAgentIPCKey)
}
