// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

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
	pdhQuery *pdhutil.PdhQuery
	// maps metric to counter object
	counters map[string]pdhutil.PdhSingleInstanceCounter
}

// Run executes the check
func (c *processChk) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Fetch PDH query values
	err = c.pdhQuery.CollectQueryData()
	if err == nil {
		// Get values for PDH counters
		for metricname, counter := range c.counters {
			var val float64
			val, err = counter.GetValue()
			if err == nil {
				sender.Gauge(metricname, val, "", nil)
			} else {
				c.Warnf("winproc.Check: Could not retrieve value for %v: %v", metricname, err)
			}
		}
	} else {
		c.Warnf("winproc.Check: Could not collect performance counter data: %v", err)
	}

	sender.Commit()
	return nil
}

func (c *processChk) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source)
	if err != nil {
		return err
	}

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counters = map[string]pdhutil.PdhSingleInstanceCounter{
		"system.proc.count":        c.pdhQuery.AddEnglishSingleInstanceCounter("System", "Processes"),
		"system.proc.queue_length": c.pdhQuery.AddEnglishSingleInstanceCounter("System", "Processor Queue Length"),
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
