// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package winproc

import (
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
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	var val float64

	// counter ("System", "Processes")
	if c.numprocs == nil {
		c.numprocs, err = pdhutil.GetEnglishSingleInstanceCounter("System", "Processes")
	}
	if c.numprocs != nil {
		val, err = c.numprocs.GetValue()
	}
	if err == nil {
		sender.Gauge("system.proc.count", val, "", nil)
	} else {
		c.Warnf("winproc.Check: Error getting number of processes: %v", err)
	}

	// counter ("System", "Processor Queue Length")
	if c.pql == nil {
		c.pql, err = pdhutil.GetEnglishSingleInstanceCounter("System", "Processor Queue Length")
	}
	if c.pql != nil {
		val, err = c.pql.GetValue()
	}
	if err == nil {
		sender.Gauge("system.proc.queue_length", val, "", nil)
	} else {
		c.Warnf("winproc.Check: Error getting processor queue length: %v", err)
	}

	sender.Commit()
	return nil
}

func (c *processChk) Configure(data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(initConfig, data, source)
	if err != nil {
		return err
	}

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
