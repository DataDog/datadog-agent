// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	dimod "github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

// DynamicInstrumentation is a system probe module which allows you to add instrumentation into
// running Go services without restarts.
var DynamicInstrumentation = module.Factory{
	Name:             config.DynamicInstrumentationModule,
	ConfigNamespaces: []string{},
	Fn: func(agentConfiguration *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		config, err := dimod.NewConfig(agentConfiguration)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
		}
		m, err := dimod.NewModule(config)
		if err != nil {
			if errors.Is(err, ebpf.ErrNotImplemented) {
				return nil, module.ErrNotEnabled
			}
			return nil, err
		}

		return m, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}
