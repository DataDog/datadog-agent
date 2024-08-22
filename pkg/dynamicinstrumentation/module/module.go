// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"net/http"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/utils"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	di "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
)

type Module struct {
	godi *di.GoDI
}

func NewModule(config *Config) (*Module, error) {
	godi, err := di.RunDynamicInstrumentation(&di.DIOptions{
		Offline:          coreconfig.SystemProbe().GetBool("dynamic_instrumentation.offline_mode"),
		ProbesFilePath:   coreconfig.SystemProbe().GetString("dynamic_instrumentation.probes_file_path"),
		SnapshotOutput:   coreconfig.SystemProbe().GetString("dynamic_instrumentation.snapshot_output_file_path"),
		DiagnosticOutput: coreconfig.SystemProbe().GetString("dynamic_instrumentation.diagnostics_output_file_path"),
	})
	if err != nil {
		return nil, err
	}
	return &Module{godi}, nil
}

func (m *Module) Close() {
	if m.godi == nil {
		log.Info("Could not close dynamic instrumentation module, already closed")
		return
	}
	log.Info("Closing dynamic instrumentation module")
	m.godi.Close()
}

func (m *Module) GetStats() map[string]interface{} {
	if m == nil || m.godi == nil {
		log.Info("Could not get stats from dynamic instrumentation module, closed")
		return map[string]interface{}{}
	}
	debug := map[string]interface{}{}
	stats := m.godi.GetStats()
	debug["PIDEventsCreated"] = stats.PIDEventsCreatedCount
	debug["ProbeEventsCreated"] = stats.ProbeEventsCreatedCount
	return debug
}

func (m *Module) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests,
		func(w http.ResponseWriter, req *http.Request) {
			stats := []string{}
			utils.WriteAsJSON(w, stats)
		}))

	log.Info("Registering dynamic instrumentation module")
	return nil
}
