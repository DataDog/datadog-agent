// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.
// +build windows

package system

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/aggregator"

	"github.com/lxn/win"
)

const fileHandlesCheckName = "file_handle"

type fhCheck struct {
	core.CheckBase
}

// Run executes the check
func (c *fhCheck) Run() error {
	var hq win.PDH_HQUERY
	var counter win.PDH_HCOUNTER
	userdata := uintptr(0)
	winerror := win.PdhOpenQuery(0, userdata, &hq)
	if win.ERROR_SUCCESS != winerror {
		return fmt.Errorf("Unable to open query %d", winerror)
	}
	defer win.PdhCloseQuery(hq)

	winerror = win.PdhAddEnglishCounter(hq, "\\Process(_Total)\\Handle Count", userdata, &counter)
	if win.ERROR_SUCCESS != winerror {
		return fmt.Errorf("Unable to add counter %d", winerror)
	}
	winerror = win.PdhCollectQueryData(hq)
	if win.ERROR_SUCCESS != winerror {
		return fmt.Errorf("Unable to collect data %d", winerror)
	}
	var value win.PDH_FMT_COUNTERVALUE_LARGE
	var dwtype uint32
	winerror = win.PdhGetFormattedCounterValueLarge(counter, &dwtype, &value)
	if win.ERROR_SUCCESS != winerror {
		return fmt.Errorf("Unable to collect value %d", winerror)
	}

	sender, err := aggregator.GetSender(c.ID())
	if err != nil {
		return err
	}
	log.Debugf("Submitting system.fs.file_handles_in_use %v", value.LargeValue)
	sender.Gauge("system.fs.file_handles.in_use", float64(value.LargeValue), "", nil)
	sender.Commit()

	return nil
}

// The check doesn't need configuration
func (c *fhCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	return nil
}

func fhFactory() check.Check {
	return &fhCheck{
		CheckBase: core.NewCheckBase(fileHandlesCheckName),
	}
}

func init() {
	core.RegisterCheck(fileHandlesCheckName, fhFactory)
}
