// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
)

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = module.Factory{
	Name:             config.NetworkTracerModule,
	ConfigNamespaces: networkTracerModuleConfigNamespaces,
	Fn:               createNetworkTracerModule,
	NeedsEBPF: func() bool {
		return !coreconfig.SystemProbe.GetBool("network_config.enable_ebpf_less")
	},
}
