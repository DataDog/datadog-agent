// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentmemprof is a core check that can capture a memory profile of the
// core agent when the core agent's memory usage exceeds a certain threshold.
package agentmemprof

import (
	"fmt"
	"runtime"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"

	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/flare/helpers"
	"github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	CheckName = "agentmemprof"
)

// AgentMemProfConfig is the configuration for the agentmemprof check
type AgentMemProfConfig struct {
	MemoryThreshold int `yaml:"memory_threshold"`
	TicketID        int `yaml:"ticket_id"`
}

// AgentMemProfCheck is the check that captures a memory profile of the core agent
type AgentMemProfCheck struct {
	core.CheckBase
	instance        *AgentMemProfConfig
	profileCaptured bool
	flareComponent  flare.Component
}

func Factory(flareComponent flare.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(flareComponent)
	})
}

func newCheck(flareComponent flare.Component) check.Check {
	return &AgentMemProfCheck{
		CheckBase:      core.NewCheckBase(CheckName),
		instance:       &AgentMemProfConfig{},
		flareComponent: flareComponent,
	}
}

func (c *AgentMemProfConfig) Parse(data []byte) error {
	// default values
	c.MemoryThreshold = 0
	c.TicketID = 0
	return yaml.Unmarshal(data, c)
}

func (m *AgentMemProfCheck) Configure(senderManager sender.SenderManager, _ uint64, config, initConfig integration.Data, source string) error {
	err := m.CommonConfigure(senderManager, initConfig, config, source)
	if err != nil {
		return err
	}

	return m.instance.Parse(config)
}

func (m *AgentMemProfCheck) Run() error {
	// Don't run again if the profile has already been captured
	if m.profileCaptured {
		log.Infof("Memory profile already captured, skipping further checks.")
		return nil
	}

	// Get the memory profile threshold from config
	thresholdBytes := m.instance.MemoryThreshold
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
		log.Infof("Heap usage exceeds threshold, generating flare with profiles.")

		err := m.generateFlare()
		if err != nil {
			log.Errorf("Failed to generate flare: %s", err)
			return err
		}

		log.Infof("Flare generated. Stopping further executions of this check.")
	}

	return nil
}

// generateFlare generates a flare and sends it to Zendesk if ticketID is specified, otherwise generates it locally
func (m *AgentMemProfCheck) generateFlare() error {
	// Prepare flare arguments
	providerTimeout := time.Duration(0) // Use default timeout

	// flareArgs := types.FlareArgs{
	// 	ProfileDuration: time.Second * 60,
	// }

	// Create an instance of the flare builder
	// flareBuilder := builder.NewFlareBuilder(flareArgs)

	// Initialize profile data
	profileData := types.ProfileData{
		"core-1st-heap.pprof": []byte("heap_profile"), // Replace with actual profile data
		"core-2nd-heap.pprof": []byte("heap_profile"),
		"core-block.pprof":    []byte("block"),
		"core-mutex.pprof":    []byte("mutex"),
	}

	// Create an instance of the flare struct
	flarePath, err := m.flareComponent.Create(profileData, providerTimeout, nil)
	if err != nil {
		return fmt.Errorf("Failed to create flare: %w", err)
	}

	if m.instance.TicketID > 0 {
		// Send the flare to Zendesk
		caseID := fmt.Sprintf("%d", m.instance.TicketID)
		userHandle := "support@datadoghq.com"
		_, err := m.flareComponent.Send(flarePath, caseID, userHandle, helpers.NewLocalFlareSource())
		if err != nil {
			return fmt.Errorf("Failed to send flare to Zendesk: %w", err)
		}
		log.Infof("Flare sent to Zendesk with case ID %d", m.instance.TicketID)
	} else {
		log.Infof("Flare generated locally at %s", flarePath)
	}

	// Mark flare as generated to stop future runs
	m.profileCaptured = true

	return nil
}
