// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build windows
// +build windows

package filehandles

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

const fileHandlesCheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
	counter *pdhutil.PdhSingleInstanceCounterSet
}

// Run executes the check
func (c *fhCheck) Run() error {

	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	var val float64

	// counter ("Process", "Handle count")
	if c.counter == nil {
		c.counter, err = pdhutil.GetEnglishCounterInstance("Process", "Handle Count", "_Total")
	}
	if c.counter != nil {
		val, err = c.counter.GetValue()
	}
	if err != nil {
		c.Warnf("file_handle.Check: Error getting process handle count: %v", err)
	} else {
		log.Debugf("Submitting system.fs.file_handles_in_use %v", val)
		sender.Gauge("system.fs.file_handles.in_use", float64(val), "", nil)
	}

	sender.Commit()
	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data integration.Data, initConfig integration.Data, source string) (err error) {
	if err := c.CommonConfigure(initConfig, data, source); err != nil {
		return err
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
