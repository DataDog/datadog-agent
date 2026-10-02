// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package native

import (
	"fmt"
	"iter"

	"github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	npmodel "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/model"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func connectionCollector(cfg config.Component, syscfg *checks.SysProbeConfig, info *checks.HostInfo, wmeta workloadmeta.Component, tagger tagger.Component) (checks.Check, error) {
	syscfg.NetworkTracerModuleEnabled = true
	c := checks.NewConnectionsCheck(cfg, cfg, &sysconfig.Config{}, wmeta, noNetworkPathTests{}, tagger)
	if err := c.Init(syscfg, info, true); err != nil {
		return nil, fmt.Errorf("cannot initialize native connections; run the matching system-probe with network_config.direct_send: false")
	}
	return c, nil
}

// Capture records existing connection evidence and must not launch active
// network-path probes against addresses observed on the native device.
type noNetworkPathTests struct{}

func (noNetworkPathTests) ScheduleNetworkPathTests(iter.Seq[npmodel.NetworkPathConnection]) {}
func (noNetworkPathTests) ScheduleNetflowPathTests(iter.Seq[npmodel.NetworkPathConnection]) {}
