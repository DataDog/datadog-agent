// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu defines the agent corecheck for
// the GPU integration
package gpu

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v2"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// CheckName defines the name of the
// GPU check
const CheckName = "gpu"

type CheckConfig struct {
}

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config        *CheckConfig
	sysProbeUtil  *processnet.RemoteSysProbeUtil
	telemetryComp telemetryComp.Component
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(CheckName),
		config:    &CheckConfig{},
	}
}

// Parse parses the check configuration
func (c *CheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure parses the check configuration and init the check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("ebpf check config: %s", err)
	}
	if err := processnet.CheckPath(ddconfig.SystemProbe().GetString("system_probe_config.sysprobe_socket")); err != nil {
		return fmt.Errorf("sysprobe socket: %s", err)
	}

	return nil
}

// Run executes the check
func (m *Check) Run() error {
	if m.sysProbeUtil == nil {
		var err error
		m.sysProbeUtil, err = processnet.GetRemoteSystemProbeUtil(
			ddconfig.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			return fmt.Errorf("sysprobe connection: %s", err)
		}
	}

	data, err := m.sysProbeUtil.GetCheck(sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("get gpu check: %s", err)
	}

	sender, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	stats, ok := data.(model.GPUStats)
	if !ok {
		return log.Errorf("ebpf check raw data has incorrect type: %T", stats)
	}

	for _, data := range stats.PastData {
		for _, span := range data.Spans {
			event := event.Event{
				SourceTypeName: CheckName,
				EventType:      "gpu-kernel",
				Title:          fmt.Sprintf("GPU Kernel launch %d", span.AvgThreadCount),
				Text:           fmt.Sprintf("Start at %d, end %d", span.Start, span.End),
				Ts:             int64(span.Start / uint64(time.Second)),
			}
			fmt.Printf("spanev: %v\n", event)
			sender.Event(event)
		}
		for _, span := range data.Allocations {
			event := event.Event{
				AlertType:      event.AlertTypeInfo,
				Priority:       event.PriorityLow,
				AggregationKey: "gpu-0",
				SourceTypeName: CheckName,
				EventType:      "gpu-memory",
				Title:          fmt.Sprintf("GPU mem alloc size %d", span.Size),
				Text:           fmt.Sprintf("Start at %d, end %d", span.Start, span.End),
				Ts:             int64(span.Start / uint64(time.Second)),
			}
			fmt.Printf("memev: %v\n", event)
			sender.Event(event)
		}
	}

	fmt.Printf("GPU stats: %+v\n", stats)

	sender.Commit()
	return nil
}
