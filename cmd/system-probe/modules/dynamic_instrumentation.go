// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	dynamicinstrumentation "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

var DynamicInstrumentation = module.Factory{
	Name:             config.DynamicInstrumentationModule,
	ConfigNamespaces: []string{},
	Fn: func(agentConfiguration *config.Config) (module.Module, error) {
		config, err := dynamicinstrumentation.NewConfig(agentConfiguration)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
		}

		m, err := dynamicinstrumentation.NewModule(config)
		if err == ebpf.ErrNotImplemented {
			return nil, module.ErrNotEnabled
		}

		return m, nil
	},
}
