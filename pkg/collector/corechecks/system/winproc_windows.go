// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.
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
	numprocs *pdhutil.PdhSingleInstanceCounterSet
	pql      *pdhutil.PdhSingleInstanceCounterSet
}

// Run executes the check
func (c *processChk) Run() error {
	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}

	procQueueLength, _ := c.pql.GetValue()
	procCount, _ := c.numprocs.GetValue()

	sender.Gauge("system.proc.queue_length", procQueueLength, "", nil)
	sender.Gauge("system.proc.count", procCount, "", nil)
	sender.Commit()

	return nil
}

func (c *processChk) Configure(data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(data, source)
	if err != nil {
		return err
	}

	c.numprocs, err = pdhutil.GetSingleInstanceCounter("System", "Processes")
	if err != nil {
		return err
	}
	c.pql, err = pdhutil.GetSingleInstanceCounter("System", "Processor Queue Length")

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
