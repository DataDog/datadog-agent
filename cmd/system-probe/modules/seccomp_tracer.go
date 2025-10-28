// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/seccomptracer"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
)

func init() { registerModule(SeccompTracer) }

// SeccompTracer Factory
var SeccompTracer = &module.Factory{
	Name:             config.SeccompTracerModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		cfg := seccomptracer.Config{
			Config:            *ebpf.NewConfig(),
			SymbolicationMode: seccomptracer.SymbolicationModeRawAddresses | seccomptracer.SymbolicationModeSymTable | seccomptracer.SymbolicationModeDWARF,
		}
		t, err := seccomptracer.NewTracer(&cfg)
		if err != nil {
			return nil, fmt.Errorf("unable to start the seccomp tracer: %w", err)
		}

		return &seccompTracerModule{
			Tracer: t,
		}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &seccompTracerModule{}

type seccompTracerModule struct {
	*seccomptracer.Tracer
	lastCheck atomic.Int64
}

func (t *seccompTracerModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", func(w http.ResponseWriter, _ *http.Request) {
		t.lastCheck.Store(time.Now().Unix())
		stats := t.Tracer.GetAndFlush()
		utils.WriteAsJSON(w, stats, utils.CompactOutput)
	})

	return nil
}

func (t *seccompTracerModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": t.lastCheck.Load(),
	}
}
