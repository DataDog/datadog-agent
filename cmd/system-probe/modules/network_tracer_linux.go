// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/tracer"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

func init() { registerModule(NetworkTracer) }

// NetworkTracer is a factory for NPM's tracer
var NetworkTracer = &module.Factory{
	Name:             config.NetworkTracerModule,
	ConfigNamespaces: networkTracerModuleConfigNamespaces,
	Fn:               createNetworkTracerModule,
	NeedsEBPF:        tracer.NeedsEBPF,
}

func inactivityEventLog(_ time.Duration) {}
