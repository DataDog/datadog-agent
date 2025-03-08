// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentmemprof is a core check that can capture a memory profile of the
// core agent when the core agent's memory usage exceeds a certain threshold.
package agentmemprof

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "agentmemprof"

// Check structure
type Check struct {
	core.CheckBase
	profileCaptured bool
	agentConfig     config.Component
}

// Factory creates a new check factory
func Factory(agentConfig config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(agentConfig)
	})
}
func newCheck(agentConfig config.Component) check.Check {
	return &Check{
		CheckBase:   core.NewCheckBase(CheckName),
		agentConfig: agentConfig,
	}
}

// Run executes the check
func (c *Check) Run() error {
	// Don't run again if the profile has already been captured
	if c.profileCaptured {
		log.Infof("Memory profile already captured, skipping further checks.")
		return nil
	}

	// Get the memory profile threshold from config
	thresholdBytes := c.agentConfig.GetInt("memory_profile_threshold")
	if thresholdBytes <= 0 {
		log.Infof("Memory profile threshold is not set or is <= 0, skipping memory profile capture check.")
		return nil
	}

	// Get Agent memory usage
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	heapUsedBytes := memStats.HeapAlloc
	heapUsedMB := float64(heapUsedBytes) / 1024 / 1024
	thresholdMB := float64(thresholdBytes) / 1024 / 1024

	log.Infof("Current heap usage: %.2f MB, Threshold: %.2f MB", heapUsedMB, thresholdMB)

	// If memory usage exceeds threshold, capture profile
	if heapUsedBytes >= uint64(thresholdBytes) {
		log.Infof("Heap usage exceeds threshold, capturing memory profile.")

		err := captureHeapProfile("/opt/datadog-agent/run/heap_profiles")
		if err != nil {
			log.Errorf("Failed to write heap profile: %s", err)
			return err
		}

		// Mark profile as captured to stop future runs
		c.profileCaptured = true
		log.Infof("Heap profile captured. Stopping further executions of this check.")
	}

	return nil
}

// captureHeapProfile writes a heap profile to a directory where flare will collect it
func captureHeapProfile(profileDir string) error {
	// Ensure the directory exists
	err := os.MkdirAll(profileDir, 0755)
	if err != nil {
		return fmt.Errorf("Failed to create directory %s: %w", profileDir, err)
	}

	// Generate a timestamped filename
	filePath := fmt.Sprintf("%s/heap-profile-%d.pprof", profileDir, time.Now().Unix())

	// Create the file
	profileFile, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("Failed to create heap profile file %s: %w", filePath, err)
	}
	defer profileFile.Close()

	// Capture the heap profile
	err = pprof.WriteHeapProfile(profileFile)
	if err != nil {
		return fmt.Errorf("Failed to write heap profile: %w", err)
	}

	log.Infof("Heap profile successfully saved at %s", filePath)
	return nil
}
