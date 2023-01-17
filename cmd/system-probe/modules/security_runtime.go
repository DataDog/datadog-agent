// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build linux
// +build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// DefaultRuntimePoliciesDir is the default policies directory used by the runtime security module
	DefaultRuntimePoliciesDir = "/etc/datadog-agent/runtime-security.d"
)

// SecurityRuntime - Security runtime Factory
var SecurityRuntime = module.Factory{
	Name:             config.SecurityRuntimeModule,
	ConfigNamespaces: []string{"runtime_security_config"},
	Fn: func(sysProbeConfig *config.Config) (module.Module, error) {
		m, err := secmodule.NewModule(sysProbeConfig)
		if err == ebpf.ErrNotImplemented {
			log.Info("Datadog runtime security agent is only supported on Linux")
			return nil, module.ErrNotEnabled
		}

		cws, err := secmodule.NewCWS(m)
		if err != nil {
			return nil, err
		}
		m.RegisterEventModule(cws)

		return m, err
	},
}
