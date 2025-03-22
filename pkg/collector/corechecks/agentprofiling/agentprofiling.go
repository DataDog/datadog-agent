// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentprofiling is a core check that can capture a memory profile of the
// core agent when the core agent's memory usage exceeds a certain threshold.
package agentprofiling

import (
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/shirou/gopsutil/v4/cpu"

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
)

// CheckName is the name of the agentprofiling check
const (
	CheckName = "agentprofiling"
	MB        = 1024 * 1024
)

// AgentProfilingConfig is the configuration for the agentprofiling check
type AgentProfilingConfig struct {
	MemoryThreshold int    `yaml:"memory_threshold"`
	CPUThreshold    int    `yaml:"cpu_threshold"`
	TicketID        string `yaml:"ticket_id"`
	UserEmail       string `yaml:"user_email"`
}

// AgentProfilingCheck is the check that captures a memory profile of the core agent
type AgentProfilingCheck struct {
	core.CheckBase
	instance        *AgentProfilingConfig
	profileCaptured bool
	flareComponent  flare.Component
	agentConfig     config.Component
}

// Factory creates a new instance of the agentprofiling check
func Factory(flareComponent flare.Component, agentConfig config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(flareComponent, agentConfig)
	})
}

// newCheck creates a new instance of the agentprofiling check
func newCheck(flareComponent flare.Component, agentConfig config.Component) check.Check {
	return &AgentProfilingCheck{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &AgentProfilingConfig{},
		flareComponent: flareComponent,
		agentConfig:    agentConfig,
	}
}

// Parse parses the configuration for the agentprofiling check
func (c *AgentProfilingConfig) Parse(data []byte) error {
	// default values
	c.MemoryThreshold = 0
	c.CPUThreshold = 0
	c.TicketID = ""
	c.UserEmail = ""
	return yaml.Unmarshal(data, c)
}

// Configure configures the agentprofiling check
func (m *AgentProfilingCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := m.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	return m.instance.Parse(config)
}

// Run runs the agentprofiling check
func (m *AgentProfilingCheck) Run() error {
	// Don't run again if the profile has already been captured
	if m.profileCaptured {
		log.Debugf("Memory profile already captured, skipping further checks.")
		return nil
	}

	// Exit early if both thresholds are disabled
	if m.instance.MemoryThreshold <= 0 && m.instance.CPUThreshold <= 0 {
		log.Debugf("Memory and CPU profile thresholds are disabled, skipping check.")
		return nil
	}

	// Get Agent memory usage
	var heapUsedMB float64
	if m.instance.MemoryThreshold > 0 {
		var memStats runtime.MemStats
		runtime.ReadMemStats(&memStats)
		heapUsedMB = float64(memStats.HeapAlloc) / MB

		log.Infof("Memory usage check - Current: %.2f MB, Threshold: %.2f MB", heapUsedMB, float64(m.instance.MemoryThreshold)/MB)
	}

	// Get Agent CPU usage
	var currentCPU float64
	if m.instance.CPUThreshold > 0 {
		percentages, err := cpu.Percent(200*time.Millisecond, false)
		if err != nil {
			log.Errorf("Failed to get CPU usage: %s", err)
			return err
		}
		currentCPU = percentages[0]

		log.Infof("CPU usage check - Current: %.2f%%, Threshold: %d%%", currentCPU, m.instance.CPUThreshold)
	}

	// Exit early if usage is below thresholds
	if heapUsedMB < float64(m.instance.MemoryThreshold)/MB && currentCPU < float64(m.instance.CPUThreshold) {
		log.Debugf("Memory and CPU usage are below thresholds (Memory: %.2f MB < %.2f MB, CPU: %.2f%% < %d%%), skipping Agent profiling check.", heapUsedMB, float64(m.instance.MemoryThreshold)/MB, currentCPU, m.instance.CPUThreshold)
		return nil
	}

	// If either memory or CPU exceeds threshold, generate flare
	log.Infof("Threshold exceeded - Memory: %.2f MB, CPU: %.2f%%. Generating flare.", heapUsedMB, currentCPU)
	if err := m.generateFlare(); err != nil {
		log.Errorf("Failed to generate flare: %s", err)
		return err
	}

	return nil
}

// generateFlare generates a flare and sends it to Zendesk if ticketID is specified, otherwise generates it locally
func (m *AgentProfilingCheck) generateFlare() error {
	// Prepare flare arguments
	providerTimeout := time.Duration(0) // Use default timeout
	flareArgs := types.FlareArgs{
		ProfileDuration:      m.agentConfig.GetDuration("flare.rc_profiling.profile_duration"),
		ProfileBlockingRate:  m.agentConfig.GetInt("flare.rc_profiling.blocking_rate"),
		ProfileMutexFraction: m.agentConfig.GetInt("flare.rc_profiling.mutex_fraction"),
	}

	// Create an instance of the flare struct
	flarePath, err := m.flareComponent.CreateWithArgs(flareArgs, providerTimeout, nil, []byte{})
	if err != nil {
		return fmt.Errorf("Failed to create flare: %w", err)
	}

	if m.instance.TicketID != "" {
		// Send the flare to Zendesk
		caseID := m.instance.TicketID
		userHandle := m.instance.UserEmail
		response, err := m.flareComponent.Send(flarePath, caseID, userHandle, helpers.NewLocalFlareSource())
		if err != nil {
			// Add debugging logs to capture the response from Zendesk
			log.Errorf("Zendesk response: %s", response)
			return fmt.Errorf("Failed to send flare to Zendesk: %w", err)
		}
		log.Infof("Flare sent to Zendesk with case ID %s", m.instance.TicketID)
	} else {
		log.Infof("Flare generated locally at %s", flarePath)
	}

	// Mark flare as generated to stop future runs
	m.profileCaptured = true

	return nil
}
