// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

const winprocCheckName = "winproc"

type processChk struct {
	core.CheckBase
	numprocs *pdhutil.PdhCounterSet
	pql      *pdhutil.PdhCounterSet
}

// Run executes the check
func (c *processChk) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	procQueueLength, _ := c.pql.GetSingleValue()
	procCount, _ := c.numprocs.GetSingleValue()

	sender.Gauge("system.proc.queue_length", procQueueLength, "", nil)
	sender.Gauge("system.proc.count", procCount, "", nil)
	sender.Commit()

	return nil
}

func (c *processChk) Configure(data integration.Data, initConfig integration.Data) error {
	err := c.CommonConfigure(data)
	if err != nil {
		return err
	}

	c.numprocs, err = pdhutil.GetCounterSet("System", "Processes", "", nil)
	if err != nil {
		return err
	}
	c.pql, err = pdhutil.GetCounterSet("System", "Processor Queue Length", "", nil)

	return err
}

func processCheckFactory() check.Check {
	return &processChk{
		CheckBase: core.NewCheckBase(winprocCheckName),
	}
}

func init() {
	core.RegisterCheck(winprocCheckName, processCheckFactory)
}
