// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentprofiling is a core check that can generate a flare with profiles
// when the core agent's memory or CPU usage exceeds a certain threshold.
package agentprofiling

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"

	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/size"
)

// CheckName is the name of the agentprofiling check
const (
	CheckName = "agentprofiling"
	MB        = 1024 * 1024
)

// Config is the configuration for the agentprofiling check
type Config struct {
	MemoryThreshold string `yaml:"memory_threshold"`
	CPUThreshold    int    `yaml:"cpu_threshold"`
	TicketID        string `yaml:"ticket_id"`
	UserEmail       string `yaml:"user_email"`
}

// Check is the check that generates a flare with profiles when the core agent's memory or CPU usage exceeds a certain threshold
type Check struct {
	core.CheckBase
	instance        *Config
	flareGenerated  bool
	flareComponent  flare.Component
	agentConfig     config.Component
	lastCPUTimes    *cpu.TimesStat
	lastCheckTime   time.Time
	memoryThreshold uint // Parsed memory threshold in bytes
}

// Factory creates a new instance of the agentprofiling check
func Factory(flareComponent flare.Component, agentConfig config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(flareComponent, agentConfig)
	})
}

// newCheck creates a new instance of the agentprofiling check
func newCheck(flareComponent flare.Component, agentConfig config.Component) check.Check {
	return &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{},
		flareComponent: flareComponent,
		agentConfig:    agentConfig,
	}
}

// Parse parses the configuration for the agentprofiling check
func (c *Config) Parse(data []byte) error {
	return yaml.Unmarshal(data, c)
}

// Configure configures the agentprofiling check
func (m *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := m.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	err = m.instance.Parse(config)
	if err != nil {
		return err
	}

	// Parse memory threshold
	m.memoryThreshold = size.ParseSizeInBytes(m.instance.MemoryThreshold)
	return nil
}

// calculateCPUPercentage calculates the CPU percentage since the last check
func (m *Check) calculateCPUPercentage(currentTimes *cpu.TimesStat) float64 {
	if m.lastCPUTimes == nil {
		m.lastCPUTimes = currentTimes
		m.lastCheckTime = time.Now()
		return 0.0
	}

	// Calculate time elapsed since last check
	elapsed := time.Since(m.lastCheckTime).Seconds()
	if elapsed <= 0 {
		return 0.0
	}

	// Calculate CPU time delta
	userDelta := currentTimes.User - m.lastCPUTimes.User
	systemDelta := currentTimes.System - m.lastCPUTimes.System
	totalDelta := userDelta + systemDelta

	// Calculate percentage
	cpuPercent := (totalDelta / elapsed) * 100.0

	// Update last values
	m.lastCPUTimes = currentTimes
	m.lastCheckTime = time.Now()

	return cpuPercent
}

// Run executes the agent profiling check, generating a flare with profiles if thresholds are exceeded
func (m *Check) Run() error {
	// Don't run again if the flare has already been generated
	if m.flareGenerated {
		return nil
	}

	// Exit early if both thresholds are disabled
	if m.memoryThreshold == 0 && m.instance.CPUThreshold <= 0 {
		m.Warn("Memory and CPU profile thresholds are disabled, skipping check.")
		return nil
	}

	// Get Agent memory usage
	var processMemoryMB float64
	if m.memoryThreshold > 0 {
		// Get the current process (agent)
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			return fmt.Errorf("Failed to get agent process: %w", err)
		}

		// Get process memory info
		memInfo, err := p.MemoryInfo()
		if err != nil {
			return fmt.Errorf("Failed to get agent memory info: %w", err)
		}

		// RSS (Resident Set Size) represents the total memory allocated to the process
		// This includes all memory: Go heap, native libraries, and other allocations
		processMemoryMB = float64(memInfo.RSS) / MB
	}

	// Get Agent CPU usage
	var currentCPU float64
	if m.instance.CPUThreshold > 0 {
		// Get the current process (agent)
		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			return fmt.Errorf("Failed to get agent process: %w", err)
		}

		// Get the total CPU time used by the process
		cpuTimes, err := p.Times()
		if err != nil {
			return fmt.Errorf("Failed to get agent CPU times: %w", err)
		}

		// Calculate CPU percentage since last check
		currentCPU = m.calculateCPUPercentage(cpuTimes)
	}

	// Exit early if usage is below thresholds
	if processMemoryMB < float64(m.memoryThreshold)/MB && currentCPU < float64(m.instance.CPUThreshold) {
		log.Debugf("Memory and CPU usage are below thresholds (Memory: %.2f MB < %.2f MB, CPU: %.2f%% < %d%%), skipping Agent profiling check.", processMemoryMB, float64(m.memoryThreshold)/MB, currentCPU, m.instance.CPUThreshold)
		return nil
	}

	// If either memory or CPU exceeds threshold, generate flare
	log.Infof("Threshold exceeded - Memory: %.2f MB, CPU: %.2f%%. Generating flare.", processMemoryMB, currentCPU)
	if err := m.generateFlare(); err != nil {
		return fmt.Errorf("Failed to generate flare: %w", err)
	}

	return nil
}

// generateFlare generates a flare and sends it to Zendesk if ticketID is specified, otherwise generates it locally
func (m *Check) generateFlare() error {
	// Skip flare generation if flareComponent is not available
	if m.flareComponent == nil {
		log.Info("Skipping flare generation: flare component not available")
		m.flareGenerated = true
		return nil
	}

	// Prepare flare arguments
	flareArgs := types.FlareArgs{
		ProfileDuration:      m.agentConfig.GetDuration("flare.rc_profiling.profile_duration"),
		ProfileBlockingRate:  m.agentConfig.GetInt("flare.rc_profiling.blocking_rate"),
		ProfileMutexFraction: m.agentConfig.GetInt("flare.rc_profiling.mutex_fraction"),
	}

	// Create an instance of the flare struct
	flarePath, err := m.flareComponent.CreateWithArgs(flareArgs, 0, nil, []byte{})
	if err != nil {
		return fmt.Errorf("Failed to create flare: %w", err)
	}

	if m.instance.TicketID != "" && m.instance.UserEmail != "" {
		// Send the flare
		caseID := m.instance.TicketID
		userHandle := m.instance.UserEmail
		response, err := m.flareComponent.Send(flarePath, caseID, userHandle, helpers.NewLocalFlareSource())
		if err != nil {
			// Include the user-friendly response message in the error
			return fmt.Errorf("Failed to send flare: %s (%w)", response, err)
		}
		log.Infof("Flare sent with case ID %q", m.instance.TicketID)
	} else {
		log.Infof("Flare generated locally at %q", flarePath)
	}

	// Mark flare as generated to stop future runs
	m.flareGenerated = true
	log.Info("Flare generation complete. No more flares will be generated until the Agent is restarted.")

	return nil
}
