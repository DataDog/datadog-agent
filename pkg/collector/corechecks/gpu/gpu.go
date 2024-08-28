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

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	telemetryComp "github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
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
	config         *CheckConfig
	sysProbeUtil   *processnet.RemoteSysProbeUtil
	telemetryComp  telemetryComp.Component
	lastCheckTime  time.Time
	timeResolver   *sectime.Resolver
	statProcessors map[uint32]*StatsProcessor
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		config:         &CheckConfig{},
		statProcessors: make(map[uint32]*StatsProcessor),
	}
}

// Parse parses the check configuration
func (c *CheckConfig) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

func (c *Check) Cancel() {
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
	if err := processnet.CheckPath(pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")); err != nil {
		return fmt.Errorf("sysprobe socket: %s", err)
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

	if m.timeResolver == nil {
		m.timeResolver, err = sectime.NewResolver()
		if err != nil {
			return fmt.Errorf("cannot create time resolver: %s", err)
		}
	}
	return nil
}

func (m *Check) ensureProcessor(key *model.StreamKey, snd sender.Sender, gpuThreads int, checkDuration time.Duration) {
	if _, ok := m.statProcessors[key.Pid]; !ok {
		m.statProcessors[key.Pid] = &StatsProcessor{
			key: key,
		}
	}

	m.statProcessors[key.Pid].totalThreadSecondsUsed = 0
	m.statProcessors[key.Pid].sender = snd
	m.statProcessors[key.Pid].gpuMaxThreads = gpuThreads
	m.statProcessors[key.Pid].measuredInterval = checkDuration
	m.statProcessors[key.Pid].timeResolver = m.timeResolver
	m.statProcessors[key.Pid].lastCheck = m.lastCheckTime
}

// Run executes the check
func (m *Check) Run() error {
	if err := m.ensureInitialized(); err != nil {
		return err
	}

	gpuDevices, err := getGPUDevices()
	if err != nil {
		return fmt.Errorf("get GPU devices: %s", err)
	}

	for _, device := range gpuDevices {
		cores, err := device.GetMaxThreads()
		if err != nil {
			return fmt.Errorf("get GPU device cores: %s", err)
		}
		name, ret := device.GetName()
		if err := wrapNvmlError(ret); err != nil {
			return fmt.Errorf("get GPU device name: %s", err)
		}
		fmt.Printf("GPU device %s has %d cores\n", name, cores)
	}

	data, err := m.sysProbeUtil.GetCheck(sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("get gpu check: %s", err)
	}
	now := time.Now()

	var checkDuration time.Duration
	// mark the check duration as close to the actual check as possible
	if !m.lastCheckTime.IsZero() {
		checkDuration = now.Sub(m.lastCheckTime)
	}

	snd, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %s", err)
	}

	stats, ok := data.(model.GPUStats)
	if !ok {
		return log.Errorf("ebpf check raw data has incorrect type: %T", stats)
	}

	gpuThreads, err := gpuDevices[0].GetMaxThreads()
	if err != nil {
		return fmt.Errorf("get GPU device threads: %s", err)
	}

	usedProcessors := make(map[uint32]bool)

	for _, data := range stats.CurrentData {
		m.ensureProcessor(&data.Key, snd, gpuThreads, checkDuration)
		m.statProcessors[data.Key.Pid].processCurrentData(data)
		usedProcessors[data.Key.Pid] = true
	}

	for _, data := range stats.PastData {
		m.ensureProcessor(&data.Key, snd, gpuThreads, checkDuration)
		m.statProcessors[data.Key.Pid].processPastData(data)
		usedProcessors[data.Key.Pid] = true
	}

	// As we compute the utilization based on the number of threads launched by the kernel, we need to
	// normalize the utlization if we get above 100%, as the GPU can enqueue threads.
	totalGPUUtilization := 0.0
	for _, processor := range m.statProcessors {
		if usedProcessors[processor.key.Pid] {
			totalGPUUtilization += processor.getGPUUtilization()
		}
	}
	normFactor := max(1.0, totalGPUUtilization)

	for _, processor := range m.statProcessors {
		if usedProcessors[processor.key.Pid] {
			processor.setGPUUtilizationNormalizationFactor(normFactor)
			processor.markInterval(now)
		} else {
			processor.finish(now)
			delete(m.statProcessors, processor.key.Pid)
		}
	}

	fmt.Printf("GPU stats: %+v\n", stats)
	snd.Commit()
	fmt.Printf("GPU check done, sender stats: %+v\n", snd.GetSenderStats())
	m.lastCheckTime = now
	return nil
}
