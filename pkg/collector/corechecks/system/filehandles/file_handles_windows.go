// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows

package filehandles

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

const fileHandlesCheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
	pdhQuery *pdhutil.PdhQuery
	// maps metric to counter object
	counters map[string]pdhutil.PdhSingleInstanceCounter
}

// Run executes the check
func (c *fhCheck) Run() error {

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
				c.Warnf("file_handle.Check: Error getting process handle count: %v", err)
			}
		}
	} else {
		c.Warnf("file_handle.Check: Could not collect performance counter data: %v", err)
	}

	sender.Commit()
	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counters = map[string]pdhutil.PdhSingleInstanceCounter{
		"system.fs.file_handles.in_use": c.pdhQuery.AddEnglishCounterInstance("Process", "Handle Count", "_Total"),
	}

	return err
}

func fhFactory() check.Check {
	return &fhCheck{
		CheckBase: core.NewCheckBase(fileHandlesCheckName),
	}
}

func init() {
	core.RegisterCheck(fileHandlesCheckName, fhFactory)
}
