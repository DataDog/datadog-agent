// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dynamicinstrumentation

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
)

type Module struct{}

func NewModule(config *Config) (*Module, error) {
	return &Module{}, nil
}

func (m *Module) Close() {
	log.Info("Closing user tracer module")
}

func (m *Module) GetStats() map[string]interface{} {
	debug := map[string]interface{}{}
	return debug
}

func (m *Module) Register(_ *module.Router) error {
	log.Info("Registering dynamic instrumentation module")
	return nil
}
