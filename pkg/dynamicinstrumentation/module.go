// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dynamicinstrumentation

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	di "github.com/DataDog/go-dynamic-instrumentation/pkg/di"
)

//nolint:revive // TODO(DEBUG) Fix revive linter
type Module struct {
	godi *di.GoDI
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func NewModule(config *Config) (*Module, error) {
	godi, err := di.RunDynamicInstrumentation(&di.DIOptions{})
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
	debug := map[string]interface{}{}
	return debug
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Register(_ *module.Router) error {
	log.Info("Registering dynamic instrumentation module")
	return nil
}
