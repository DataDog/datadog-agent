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

	di "github.com/DataDog/datadog-agent/pkg/di"
)

//nolint:revive // TODO(DEBUG) Fix revive linter
type Module struct {
	godi *di.GoDI
}

//nolint:revive // TODO(DEBUG) Fix revive linter
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

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Close() {
	log.Info("Closing user tracer module")
	m.godi.Close()
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) GetStats() map[string]interface{} {
	m.godi.GetStats()
	debug := map[string]interface{}{}
	return debug
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(utils.DefaultMaxConcurrentRequests,
		func(w http.ResponseWriter, req *http.Request) {
			stats := []string{}
			utils.WriteAsJSON(w, stats)
		}))

	log.Info("Registering dynamic instrumentation module")
	return nil
}
