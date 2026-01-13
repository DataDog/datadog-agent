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
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cenkalti/backoff/v5"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/DataDog/datadog-agent/cmd/agent/common/signals"
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
	CheckName               = "agentprofiling"
	MB                      = 1024 * 1024
	defaultMaxFlareAttempts = 5 // Default maximum number of flare attempts
)

// Config is the configuration for the agentprofiling check
type Config struct {
	MemoryThreshold           string `yaml:"memory_threshold"`
	CPUThreshold              int    `yaml:"cpu_threshold"`
	TicketID                  string `yaml:"ticket_id"`
	UserEmail                 string `yaml:"user_email"`
	TerminateAgentOnThreshold bool   `yaml:"terminate_agent_on_threshold"`
}

// Check is the check that generates a flare with profiles when the core agent's memory or CPU usage exceeds a certain threshold
type Check struct {
	core.CheckBase
	instance          *Config
	flareAttempted    bool
	flareAttemptCount int       // Number of flare attempts made
	lastFlareAttempt  time.Time // Time of last flare attempt
	backoffPolicy     backoff.BackOff
	flareComponent    flare.Component
	agentConfig       config.Component
	lastCPUTimes      *cpu.TimesStat
	lastCheckTime     time.Time
	memoryThreshold   uint // Parsed memory threshold in bytes
}

// Factory creates a new instance of the agentprofiling check
func Factory(flareComponent flare.Component, agentConfig config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(flareComponent, agentConfig)
	})
}

// newCheck creates a new instance of the agentprofiling check
func newCheck(flareComponent flare.Component, agentConfig config.Component) check.Check {
	// Configure exponential backoff for flare generation retries.
	// Flare generation is expensive, so we use a conservative backoff:
	// - Start at 2 minutes to allow transient issues to resolve
	// - Double each retry (standard exponential backoff)
	// - Cap at 10 minutes to avoid excessive delays
	// With 5 total attempts, backoff durations between retries are:
	// - After attempt 1 fails: ~2min before attempt 2
	// - After attempt 2 fails: ~4min before attempt 3
	// - After attempt 3 fails: ~8min before attempt 4
	// - After attempt 4 fails: ~10min (capped) before attempt 5
	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 2 * time.Minute
	expBackoff.MaxInterval = 10 * time.Minute
	expBackoff.Multiplier = 2.0          // Standard exponential backoff
	expBackoff.RandomizationFactor = 0.1 // Small randomization to avoid thundering herd
	expBackoff.Reset()

	return &Check{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &Config{},
		backoffPolicy:  expBackoff,
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
	// Don't run again if we've exhausted all retry attempts
	if m.flareAttempted {
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

	// If either memory or CPU exceeds threshold, attempt to generate flare
	attemptNumber := m.flareAttemptCount + 1
	log.Infof("Threshold exceeded - Memory: %.2f MB, CPU: %.2f%%. Attempting flare generation (attempt %d/%d).", processMemoryMB, currentCPU, attemptNumber, defaultMaxFlareAttempts)

	// Reset backoff policy when we first detect threshold exceeded (first attempt)
	if m.flareAttemptCount == 0 {
		m.backoffPolicy.Reset()
	}

	// Check if we need to wait before retrying (exponential backoff)
	// For retries (attempt > 1), check if enough time has passed since last attempt
	if m.flareAttemptCount > 0 {
		// Get the backoff duration for the next retry attempt
		// This advances the backoff policy state, which is correct since we're about to retry
		backoffDuration := m.backoffPolicy.NextBackOff()
		if backoffDuration == backoff.Stop {
			// Backoff policy indicates we should stop retrying
			m.flareAttempted = true
			log.Warnf("Flare generation backoff policy indicates stop. No more flares will be generated until the Agent is restarted.")
			return nil
		}

		timeSinceLastAttempt := time.Since(m.lastFlareAttempt)
		if timeSinceLastAttempt < backoffDuration {
			remainingWait := backoffDuration - timeSinceLastAttempt
			log.Debugf("Waiting %v before retry (backoff: %v, attempt %d/%d)", remainingWait, backoffDuration, attemptNumber, defaultMaxFlareAttempts)
			return nil // Skip this run, will retry on next check interval
		}
	}

	// Increment attempt count and record attempt time before attempting
	m.flareAttemptCount++
	m.lastFlareAttempt = time.Now()

	if err := m.generateFlare(); err != nil {
		// Check if we've exhausted retries
		if m.flareAttemptCount >= defaultMaxFlareAttempts {
			m.flareAttempted = true
			log.Warnf("Flare generation failed after %d attempts. No more flares will be generated until the Agent is restarted. Last error: %v", defaultMaxFlareAttempts, err)
			return nil // Don't return error to avoid check failure spam
		}

		log.Warnf("Flare generation attempt %d/%d failed: %v. Will retry with backoff.", m.flareAttemptCount, defaultMaxFlareAttempts, err)
		return nil // Don't return error to avoid check failure spam
	}

	// Success - reset backoff policy for future use
	m.backoffPolicy.Reset()
	return nil
}

// terminateAgent requests graceful shutdown of the agent process after flare generation completes.
// It uses the agent's established shutdown mechanism (signals.Stopper) which ensures proper cleanup
// via stopAgent(). Termination is skipped when running in test mode to avoid killing the test process.
func (m *Check) terminateAgent() {
	// Skip termination when running in test mode
	if testing.Testing() {
		log.Info("Skipping agent termination: running in test mode")
		return
	}

	log.Warnf("Terminating agent process due to threshold exceeded (terminate_agent_on_threshold is enabled)")

	// Flush logs to ensure termination message is written before triggering shutdown
	log.Flush()

	// Use the agent's established shutdown mechanism to trigger graceful shutdown.
	// This ensures all cleanup happens properly via stopAgent() in command.go.
	// The channel is unbuffered, but since the agent's run() function sets up a listener
	// before starting the agent, this is safe. If the channel is not being listened to
	// (e.g., in tests), this will block, but we've already checked for test mode above.
	signals.Stopper <- true
	log.Info("Agent Profiling check: Graceful shutdown requested. Agent will exit after cleanup.")
	log.Flush()
}

// generateFlare generates a flare and sends it to Zendesk if ticketID is specified, otherwise generates it locally
func (m *Check) generateFlare() error {
	// Skip flare generation if flareComponent is not available
	if m.flareComponent == nil {
		log.Info("Skipping flare generation: flare component not available")
		m.flareAttempted = true
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
		log.Infof("Flare sent successfully with case ID %q (attempt %d/%d)", m.instance.TicketID, m.flareAttemptCount, defaultMaxFlareAttempts)
	} else {
		log.Infof("Flare generated locally at %q (attempt %d/%d)", flarePath, m.flareAttemptCount, defaultMaxFlareAttempts)
	}

	// Success! Mark as attempted to prevent further attempts
	m.flareAttempted = true
	log.Info("Flare generation successful. No more flares will be generated until the Agent is restarted.")

	// Terminate agent if configured to do so
	if m.instance.TerminateAgentOnThreshold {
		m.terminateAgent()
	}

	return nil
}
