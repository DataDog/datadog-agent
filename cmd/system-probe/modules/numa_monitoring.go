// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/numamonitoring"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(NUMAMonitoring) }

// NUMAMonitoring is the capability-driven NUMA monitoring module factory.
var NUMAMonitoring = &module.Factory{
	Name: config.NUMAMonitoringModule,
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		maxGroups := pkgconfigsetup.SystemProbe().GetInt("numa_monitoring.max_resctrl_groups")
		probe, err := numamonitoring.NewProbe(ebpf.NewConfig(), maxGroups)
		if err != nil {
			return nil, fmt.Errorf("start NUMA monitoring probe: %w", err)
		}
		log.Infof("Starting NUMA monitoring module with resctrl capacity %d", maxGroups)
		return &numaMonitoringModule{Probe: probe}, nil
	},
	NeedsEBPF: func() bool { return true },
}

type numaMonitoringModule struct {
	*numamonitoring.Probe
	lastCheck atomic.Int64
}

var _ module.Module = (*numaMonitoringModule)(nil)

func (module *numaMonitoringModule) GetStats() map[string]interface{} {
	status := module.Status()
	return map[string]interface{}{
		"last_check":             module.lastCheck.Load(),
		"state":                  status.State,
		"architecture":           status.Architecture,
		"numa_nodes":             status.NUMANodes,
		"monitor_features":       status.MonitorFeatures,
		"active_groups":          status.ActiveGroups,
		"capacity":               status.Capacity,
		"foreign_task_conflicts": status.ForeignTaskConflicts,
		"read_failures":          status.ReadFailures,
		"message":                status.Message,
	}
}

func (module *numaMonitoringModule) Register(router *module.Router) error {
	router.HandleFunc("/check", utils.WithConcurrencyLimit(1, func(writer http.ResponseWriter, request *http.Request) {
		module.lastCheck.Store(time.Now().Unix())
		utils.WriteAsJSON(request, writer, module.GetAndFlush(), utils.GetPrettyPrintFromQueryParams(request))
	}))
	return nil
}
