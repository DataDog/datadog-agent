// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package modules

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	sconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SecurityRuntime - Security runtime Factory
var SecurityRuntime = api.Factory{
	Name: "security_runtime",
	Fn: func(agentConfig *config.AgentConfig) (api.Module, error) {
		config, err := sconfig.NewConfig(agentConfig)
		if err != nil {
			return nil, errors.Wrap(err, "invalid security runtime module configuration")
		}

		if !config.IsEnabled() {
			log.Infof("security runtime module disabled")
			return nil, api.ErrNotEnabled
		}

		module, err := secmodule.NewModule(config)
		if err == ebpf.ErrNotImplemented {
			log.Info("Datadog runtime security agent is only supported on Linux")
			return nil, api.ErrNotEnabled
		}
		return module, err
	},
}
