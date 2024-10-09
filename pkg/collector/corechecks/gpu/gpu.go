// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package gpu

import (
	"fmt"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/gpu/cuda"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	config         *CheckConfig
	sysProbeUtil   *processnet.RemoteSysProbeUtil
	statProcessors map[statProcessorKey]*StatsProcessor
	probeCtx       *probeContext
}

type statProcessorKey struct {
	pid     uint32
	gpuUUID string
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		config:         &CheckConfig{},
		statProcessors: make(map[statProcessorKey]*StatsProcessor),
		probeCtx:       &probeContext{},
	}
}

// Parse parses the check configuration
func (m *CheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, m)
}

// Cancel cancels the check
func (m *Check) Cancel() {
	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		log.Warnf("Failed to shutdown NVML: %v", nvml.ErrorString(ret))
	}
}

// Configure parses the check configuration and init the check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := m.config.Parse(config); err != nil {
		return fmt.Errorf("ebpf check config: %s", err)
	}

	return nil
}

func (m *Check) ensureInitialized() error {
	var err error

	if m.sysProbeUtil == nil {
		m.sysProbeUtil, err = processnet.GetRemoteSystemProbeUtil(
			pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket"),
		)
		if err != nil {
			return fmt.Errorf("sysprobe connection: %s", err)
		}
	}

	if m.probeCtx.timeResolver == nil {
		m.probeCtx.timeResolver, err = sectime.NewResolver()
		if err != nil {
			return fmt.Errorf("cannot create time resolver: %s", err)
		}
	}
	return nil
}

func streamKeyToStatProcessorKey(key *model.StreamKey) statProcessorKey {
	return statProcessorKey{
		pid:     key.Pid,
		gpuUUID: key.GPUUUID,
	}
}

// ensureProcessor ensures that there is a stats processor for the given key
func (m *Check) ensureProcessor(key *model.StreamKey, metadata *model.StreamMetadata) {
	sKey := streamKeyToStatProcessorKey(key)

	if _, ok := m.statProcessors[sKey]; !ok {
		m.statProcessors[sKey] = &StatsProcessor{
			key:      key,
			probeCtx: m.probeCtx,
		}
	}

	// Metadata can change, so we need to update it
	m.statProcessors[sKey].metadata = metadata
}

func (m *Check) refreshContext(now time.Time) error {
	gpuDevices, err := cuda.GetGPUDevices()
	if err != nil {
		return fmt.Errorf("get GPU devices: %s", err)
	}

	if len(gpuDevices) == 0 {
		return fmt.Errorf("no GPU devices found")
	}

	// GPU devices might change, so we need to update the context. It's not
	// an expensive operation anyways.
	for _, device := range gpuDevices {
		uuid, ret := device.GetUUID()
		if err := cuda.WrapNvmlError(ret); err != nil {
			return fmt.Errorf("get GPU UUID: %s", err)
		}

		m.probeCtx.gpuDeviceMap[uuid] = device
	}

	if !m.probeCtx.lastCheck.IsZero() {
		m.probeCtx.checkDuration = now.Sub(m.probeCtx.lastCheck)
	}

	m.probeCtx.sender, err = m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	return nil
}

// Run executes the check
func (m *Check) Run() error {
	if err := m.ensureInitialized(); err != nil {
		return err
	}

	data, err := m.sysProbeUtil.GetCheck(sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("get gpu check: %s", err)
	}

	// mark the check duration as close to the actual check as possible
	now := time.Now()
	if err := m.refreshContext(now); err != nil {
		return fmt.Errorf("cannot refresh context: %s", err)
	}

	stats, ok := data.(model.GPUStats)
	if !ok {
		return log.Errorf("ebpf check raw data has incorrect type: %T", stats)
	}

	usedProcessors := make(map[statProcessorKey]bool)

	for _, data := range stats.CurrentData {
		skey := streamKeyToStatProcessorKey(&data.Key)
		m.ensureProcessor(&data.Key, &data.Metadata)
		m.statProcessors[skey].processCurrentData(data)
		usedProcessors[skey] = true
	}

	for _, data := range stats.PastData {
		skey := streamKeyToStatProcessorKey(&data.Key)
		m.ensureProcessor(&data.Key, &data.Metadata)
		m.statProcessors[skey].processPastData(data)
		usedProcessors[skey] = true
	}

	// As we compute the utilization based on the number of threads launched by the kernel, we need to
	// normalize the utlization if we get above 100%, as the GPU can enqueue threads.
	totalGPUUtilization := 0.0
	for skey, processor := range m.statProcessors {
		if usedProcessors[skey] {
			gpuUtil, err := processor.getGPUUtilization()
			if err != nil {
				return fmt.Errorf("get GPU utilization: %s", err)
			}
			totalGPUUtilization += gpuUtil
		}
	}
	normFactor := max(1.0, totalGPUUtilization)

	for skey, processor := range m.statProcessors {
		if usedProcessors[skey] {
			processor.setGPUUtilizationNormalizationFactor(normFactor)
			err := processor.markInterval()
			if err != nil {
				return fmt.Errorf("mark interval: %s", err)
			}
		} else {
			err := processor.finish(now)
			// delete even in an error case, as we don't want to keep the processor around
			delete(m.statProcessors, skey)
			if err != nil {
				return fmt.Errorf("finish processor: %s", err)
			}
		}
	}

	m.probeCtx.sender.Commit()
	m.probeCtx.lastCheck = now

	return nil
}
