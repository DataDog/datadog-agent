// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"errors"
	"github.com/shirou/gopsutil/cpu"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

const processesCheckName = "processes"

type processChk struct {
	core.CheckBase
}

// Run executes the check
func (c *processChk) Run() error {
	processesValues, err := cpu.ProcInfo()
	if err != nil {
		log.Errorf("Could not gather Process values from psutil")
		return err
	}

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	procQueueLength := processesValues.ProcessorQueueLength
	procCount := processesValues.Processes

	sender.Gauge("system.proc.queue_length", procQueueLength, "", nil)
	sender.Gauge("system.proc.count", procCount, "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *processChk) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

func processCheckFactory() check.Check {
	return &processChk{
		CheckBase: core.NewCheckBase(processesCheckName),
	}
}

func init() {
	core.RegisterCheck(processesCheckName, processCheckFactory)
}
