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
	processnet "github.com/DataDog/datadog-agent/pkg/process/net"
	sectime "github.com/DataDog/datadog-agent/pkg/security/resolvers/time"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Check represents the GPU check that will be periodically executed via the Run() function
type Check struct {
	core.CheckBase
	config         *CheckConfig                   // config for the check
	sysProbeUtil   *processnet.RemoteSysProbeUtil // sysProbeUtil is used to communicate with system probe
	lastCheckTime  time.Time                      // lastCheckTime is the time the last check was done, used to compute GPU average utilization
	timeResolver   *sectime.Resolver              // TODO: Move resolver to a common package, see EBPF-579
	statProcessors map[uint32]*StatsProcessor     // statProcessors is a map of processors, one per pid
	gpuDevices     []gpuDevice                    // gpuDevices is a list of GPU devices found in the host
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

// Cancel cancels the check
func (m *Check) Cancel() {
	m.CheckBase.Cancel()

	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS && ret != nvml.ERROR_UNINITIALIZED {
		log.Warnf("Failed to shutdown NVML: %v", nvml.ErrorString(ret))
	}
}

// Configure parses the check configuration and init the check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := m.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}
	if err := yaml.Unmarshal(config, m.config); err != nil {
		return fmt.Errorf("invalid gpu check config: %w", err)
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
			return fmt.Errorf("sysprobe connection: %w", err)
		}
	}

	if m.timeResolver == nil {
		m.timeResolver, err = sectime.NewResolver()
		if err != nil {
			return fmt.Errorf("cannot create time resolver: %w", err)
		}
	}

	if m.gpuDevices == nil {
		m.gpuDevices, err = getGPUDevices()
		if err != nil {
			return fmt.Errorf("cannot get GPU devices: %w", err)
		}
	}

	return nil
}

// ensureProcessor ensures that there is a stats processor for the given key
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
	if len(m.gpuDevices) == 0 {
		return fmt.Errorf("no GPU devices found")
	}

	sysprobeData, err := m.sysProbeUtil.GetCheck(sysconfig.GPUMonitoringModule)
	if err != nil {
		return fmt.Errorf("cannot get data from system-probe: %w", err)
	}
	now := time.Now()

	var checkDuration time.Duration
	// mark the check duration as close to the actual check as possible
	if !m.lastCheckTime.IsZero() {
		checkDuration = now.Sub(m.lastCheckTime)
	}

	snd, err := m.GetSender()
	if err != nil {
		return fmt.Errorf("get metric sender: %w", err)
	}

	// Commit the metrics even in case of an error
	defer snd.Commit()

	stats, ok := sysprobeData.(model.GPUStats)
	if !ok {
		return log.Errorf("gpu check raw data has incorrect type: %T", stats)
	}

	// TODO: Multiple GPUs are not supported yet
	gpuThreads, err := m.gpuDevices[0].GetMaxThreads()
	if err != nil {
		return fmt.Errorf("get GPU device threads: %w", err)
	}

	for _, data := range stats.CurrentData {
		m.ensureProcessor(&data.Key, snd, gpuThreads, checkDuration)
		m.statProcessors[data.Key.Pid].processCurrentData(data)
	}

	for _, data := range stats.PastData {
		m.ensureProcessor(&data.Key, snd, gpuThreads, checkDuration)
		m.statProcessors[data.Key.Pid].processPastData(data)
	}

	m.configureNormalizationFactor()

	for _, processor := range m.statProcessors {
		if processor.hasPendingData {
			err := processor.markInterval()
			if err != nil {
				return fmt.Errorf("mark interval: %s", err)
			}
		} else {
			err := processor.finish(now)
			// delete even in an error case, as we don't want to keep the processor around
			delete(m.statProcessors, processor.key.Pid)
			if err != nil {
				return fmt.Errorf("finish processor: %s", err)
			}
		}
	}

	m.lastCheckTime = now

	return nil
}

func (m *Check) configureNormalizationFactor() {
	// As we compute the utilization based on the number of threads launched by the kernel, we need to
	// normalize the utilization if we get above 100%, as the GPU can enqueue threads.
	totalGPUUtilization := 0.0
	for _, processor := range m.statProcessors {
		// Only consider processors that received data this interval
		if processor.hasPendingData {
			totalGPUUtilization += processor.getGPUUtilization()
		}
	}

	normFactor := max(1.0, totalGPUUtilization)

	for _, processor := range m.statProcessors {
		processor.setGPUUtilizationNormalizationFactor(normFactor)
	}
}
