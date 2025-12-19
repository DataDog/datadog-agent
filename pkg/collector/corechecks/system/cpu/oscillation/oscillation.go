// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux || darwin

// Package oscillation implements the CPU oscillation detection check.
package oscillation

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "cpu_oscillation"

	// emitInterval is how often we emit metrics (every 15 seconds)
	emitInterval = 15 * time.Second
)

// For testing purposes
var getCPUTimes = cpu.Times

// Check implements the CPU oscillation detection check
type Check struct {
	core.CheckBase

	detector *OscillationDetector
	instance *Config

	// Long-running check control
	stopCh chan struct{}

	// CPU sampling state
	lastCPUTimes   cpu.TimesStat
	lastSampleTime time.Time
	hasFirstSample bool
}

// Factory returns a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return core.NewLongRunningCheckWrapper(&Check{
			CheckBase: core.NewCheckBase(CheckName),
			instance:  &Config{},
			stopCh:    make(chan struct{}),
		})
	})
}

// Configure parses the check configuration and initializes the check
func (c *Check) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(senderManager, initConfig, config, source); err != nil {
		return err
	}

	if err := c.instance.Parse(config); err != nil {
		return err
	}

	// Initialize detector with configuration
	c.detector = NewOscillationDetector(c.instance.DetectorConfig())

	log.Infof("[%s] Configured with amplitude_multiplier=%.2f, min_amplitude=%.2f, warmup_seconds=%d",
		CheckName, c.instance.AmplitudeMultiplier, c.instance.MinAmplitude, c.instance.WarmupSeconds)

	return nil
}

// Run starts the CPU oscillation check - runs indefinitely until stopped
func (c *Check) Run() error {
	log.Infof("Starting long-running check %q", c.ID())
	defer log.Infof("Shutting down long-running check %q", c.ID())

	sampleTicker := time.NewTicker(time.Second) // 1Hz sampling
	defer sampleTicker.Stop()

	emitTicker := time.NewTicker(emitInterval) // Emit metrics every 15s
	defer emitTicker.Stop()

	for {
		select {
		case <-sampleTicker.C:
			c.sampleCPU()

		case <-emitTicker.C:
			c.emitMetrics()

		case <-c.stopCh:
			return nil
		}
	}
}

// sampleCPU takes a CPU sample and adds it to the detector
func (c *Check) sampleCPU() {
	cpuPercent, err := c.calculateCPUPercent()
	if err != nil {
		if !errors.Is(err, errFirstSample) {
			log.Debugf("[%s] Error sampling CPU: %v", CheckName, err)
		}
		return
	}

	c.detector.AddSample(cpuPercent)
	c.detector.DecrementWarmup()
}

var errFirstSample = errors.New("first sample, need delta")

// calculateCPUPercent calculates CPU busy percentage since last sample
func (c *Check) calculateCPUPercent() (float64, error) {
	times, err := getCPUTimes(false) // Aggregate, not per-CPU
	if err != nil {
		return 0, err
	}

	if len(times) == 0 {
		return 0, errors.New("no CPU times returned")
	}

	t := times[0]

	if !c.hasFirstSample {
		c.lastCPUTimes = t
		c.lastSampleTime = time.Now()
		c.hasFirstSample = true
		return 0, errFirstSample
	}

	// Calculate totals
	total := t.User + t.System + t.Idle + t.Nice + t.Iowait + t.Irq + t.Softirq + t.Steal
	prevTotal := c.lastCPUTimes.User + c.lastCPUTimes.System + c.lastCPUTimes.Idle +
		c.lastCPUTimes.Nice + c.lastCPUTimes.Iowait + c.lastCPUTimes.Irq +
		c.lastCPUTimes.Softirq + c.lastCPUTimes.Steal

	deltaTotal := total - prevTotal
	deltaIdle := t.Idle - c.lastCPUTimes.Idle

	c.lastCPUTimes = t
	c.lastSampleTime = time.Now()

	if deltaTotal == 0 {
		return 0, nil
	}

	busyPercent := 100.0 * (1.0 - deltaIdle/deltaTotal)
	return busyPercent, nil
}

// emitMetrics analyzes and emits oscillation metrics
func (c *Check) emitMetrics() {
	// Don't emit until window is full
	if !c.detector.IsWindowFull() {
		log.Debugf("[%s] Window not full yet, skipping metric emission", CheckName)
		return
	}

	sender, err := c.GetSender()
	if err != nil {
		log.Warnf("[%s] Error getting sender: %v", CheckName, err)
		return
	}

	result := c.detector.Analyze()

	detected := 0.0
	if result.Detected {
		detected = 1.0
	}

	sender.Gauge("system.cpu.oscillation.detected", detected, "", nil)
	sender.Gauge("system.cpu.oscillation.amplitude", result.Amplitude, "", nil)
	sender.Gauge("system.cpu.oscillation.frequency", result.Frequency, "", nil)
	sender.Gauge("system.cpu.oscillation.zero_crossings", float64(result.ZeroCrossings), "", nil)
	sender.Gauge("system.cpu.oscillation.baseline_stddev", c.detector.BaselineStdDev(), "", nil)

	// Log and emit event when oscillation is detected
	if result.Detected {
		log.Infof("[%s] Oscillation detected: amplitude=%.2f%%, frequency=%.3fHz, zero_crossings=%d, baseline_stddev=%.2f",
			CheckName, result.Amplitude, result.Frequency, result.ZeroCrossings, c.detector.BaselineStdDev())

		// Send a Datadog event
		sender.Event(event.Event{
			Title: "CPU Oscillation Detected",
			Text: fmt.Sprintf("CPU oscillation pattern detected on this host.\n\n"+
				"**Amplitude:** %.2f%%\n"+
				"**Frequency:** %.3f Hz\n"+
				"**Zero Crossings:** %d\n"+
				"**Baseline StdDev:** %.2f\n\n"+
				"This may indicate restart loops, thrashing, retry storms, or misconfigured autoscaling.",
				result.Amplitude, result.Frequency, result.ZeroCrossings, c.detector.BaselineStdDev()),
			AlertType:      event.AlertTypeWarning,
			SourceTypeName: CheckName,
			EventType:      CheckName,
		})
	}

	// Note: sender.Commit() is called by LongRunningCheckWrapper every 15 seconds
}

// Stop stops the check
func (c *Check) Stop() {
	close(c.stopCh)
}

// Interval returns 0 to indicate this is a long-running check
func (c *Check) Interval() time.Duration {
	return 0
}

// BaselineStdDev returns the current baseline standard deviation (exported for testing)
func (c *Check) BaselineStdDev() float64 {
	if c.detector == nil {
		return 0
	}
	return math.Sqrt(c.detector.baselineVariance)
}
