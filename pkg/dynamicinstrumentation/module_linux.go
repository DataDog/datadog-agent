// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dynamicinstrumentation

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

//nolint:revive // TODO(DEBUG) Fix revive linter
type Module struct{}

//nolint:revive // TODO(DEBUG) Fix revive linter
func NewModule(config *Config) (*Module, error) {
	return &Module{}, nil
}

//nolint:revive // TODO(DEBUG) Fix revive linter
func (m *Module) Close() {
	log.Info("Closing user tracer module")
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
