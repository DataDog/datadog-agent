// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const dynamicInstrumentationEnabledKey = "dynamic_instrumentation.enabled"

// watchRuntimeModuleToggles reacts to runtime config changes that enable or
// disable a module, loading or unloading it without restarting system-probe.
// Today only the Live Debugger (dynamic instrumentation) module is toggled this
// way; its config value is driven by remote config (see the AGENT_CONFIG
// consumer in comp/remote-config/rcclient).
//
// Note: enabling an eBPF module that was not enabled at boot does not re-run the
// eBPF pre-register setup; that gap is tracked separately and must be validated
// on an eBPF-capable host.
func watchRuntimeModuleToggles(cfg model.ReaderWriter, deps module.FactoryDependencies) {
	cfg.OnUpdate(func(setting string, _ model.Source, _, _ any, _ uint64) {
		if setting != dynamicInstrumentationEnabledKey {
			return
		}
		factory := findModuleFactory(config.DynamicInstrumentationModule)
		if factory == nil {
			return
		}
		// Read the effective value so a higher-priority local source still wins.
		enabled := cfg.GetBool(setting)
		// OnUpdate receivers must not block, and (dis)enabling the module is heavy.
		go func() {
			var err error
			if enabled {
				err = module.EnableModule(factory, deps)
			} else {
				err = module.DisableModule(factory.Name)
			}
			if err != nil {
				log.Errorf("failed to apply runtime toggle for module %s: %s", factory.Name, err)
			}
		}()
	})
}
