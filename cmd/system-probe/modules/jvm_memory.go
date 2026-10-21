// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package modules

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/jvmmemory"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(JVMMemory) }

// JVMMemory Factory
var JVMMemory = &module.Factory{
	Name: config.JVMMemoryModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the JVM memory module")
		p, err := jvmmemory.NewProbe(ebpf.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("unable to start the JVM memory probe: %w", err)
		}
		return &jvmMemoryModule{probe: p}, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}

var _ module.Module = &jvmMemoryModule{}

type jvmMemoryModule struct {
	probe     *jvmmemory.Probe
	lastCheck atomic.Int64
}

// GetStats implements module.Module.GetStats
func (m *jvmMemoryModule) GetStats() map[string]interface{} {
	stats := m.probe.DebugStats()
	stats["last_check"] = m.lastCheck.Load()
	return stats
}

// Register implements module.Module.Register
func (m *jvmMemoryModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(w http.ResponseWriter, req *http.Request) {
		m.lastCheck.Store(time.Now().Unix())
		utils.WriteAsJSON(req, w, m.probe.GetStats(), utils.GetPrettyPrintFromQueryParams(req))
	}))
	return nil
}

// Close implements module.Module.Close
func (m *jvmMemoryModule) Close() {
	m.probe.Close()
}
