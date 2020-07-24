// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SecurityRuntime - Security runtime Factory
var SecurityRuntime = api.Factory{
	Name: "security_runtime",
	Fn: func(cfg *config.AgentConfig) (api.Module, error) {
		module, err := secmodule.NewModule(cfg)
		if err == ebpf.ErrNotImplemented {
			log.Info("Datadog runtime security agent is only supported on Linux")
			return nil, api.ErrNotEnabled
		}
		return module, err
	},
}
