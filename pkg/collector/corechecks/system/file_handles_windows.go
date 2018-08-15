// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

const fileHandlesCheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
	counter *pdhutil.PdhCounterSet
}

// Run executes the check
func (c *fhCheck) Run() error {

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	vals, err := c.counter.GetSingleValue()
	if err != nil {
		log.Warnf("Error getting handle value %v", err)
		return err
	}
	val := vals["Handle Count"]
	log.Debugf("Submitting system.fs.file_handles_in_use %v", val)
	sender.Gauge("system.fs.file_handles.in_use", float64(val), "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data integration.Data, initConfig integration.Data) (err error) {
	c.counter, err = pdhutil.GetCounterSet("Process", []string{"Handle Count"}, "_Total", nil)
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
