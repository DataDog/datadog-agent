// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package thermal implements the thermal zone check for Windows.
package thermal

import (
	"errors"
	"strings"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/DataDog/datadog-agent/pkg/util/pdhutil"
)

const (
	// CheckName is the name of the check
	CheckName = "thermal"

	// kelvinOffset is subtracted from Kelvin to get Celsius
	kelvinOffset = 273.15
)

// thermalCheck collects thermal zone metrics via PDH
type thermalCheck struct {
	core.CheckBase
	pdhQuery          *pdhutil.PdhQuery
	temperature       pdhutil.PdhMultiInstanceCounter
	highPrecisionTemp pdhutil.PdhMultiInstanceCounter
	passiveLimit      pdhutil.PdhMultiInstanceCounter
}

// isNotTotal filters out the _Total instance
func isNotTotal(instance string) bool {
	return !strings.EqualFold(instance, "_Total")
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &thermalCheck{
		CheckBase: core.NewCheckBase(CheckName),
	}
}

// Configure initializes the check
func (c *thermalCheck) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	err := c.CommonConfigure(senderManager, initConfig, data, source, provider)
	if err != nil {
		return err
	}

	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.temperature = c.pdhQuery.AddEnglishMultiInstanceCounter("Thermal Zone Information", "Temperature", isNotTotal)
	c.highPrecisionTemp = c.pdhQuery.AddEnglishMultiInstanceCounter("Thermal Zone Information", "High Precision Temperature", isNotTotal)
	c.passiveLimit = c.pdhQuery.AddEnglishMultiInstanceCounter("Thermal Zone Information", "% Passive Limit", isNotTotal)

	return nil
}

// Run executes the check
func (c *thermalCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	err = c.pdhQuery.CollectQueryData()
	if err != nil {
		// PDH_NO_DATA is expected on systems with zero thermal zone instances
		// (e.g. VMs, build machines). Silently emit no metrics in that case.
		if errors.Is(err, windows.Errno(pdhutil.PDH_NO_DATA)) {
			sender.Commit()
			return nil
		}
		c.Warnf("thermal.Check: Could not collect performance counter data: %v", err)
		sender.Commit()
		return nil
	}

	// Collect temperature values, preferring High Precision Temperature (tenths of Kelvin)
	// over Temperature (Kelvin) when available for higher precision.
	var highPrecisionVals map[string]float64
	if c.highPrecisionTemp != nil {
		vals, err := c.highPrecisionTemp.GetAllValues()
		if err != nil {
			log.Debugf("thermal.Check: High Precision Temperature unavailable, falling back to Temperature: %v", err)
		} else {
			highPrecisionVals = vals
		}
	}

	if c.temperature != nil {
		vals, err := c.temperature.GetAllValues()
		if err != nil {
			c.Warnf("thermal.Check: Error getting temperature values: %v", err)
		} else {
			for instance, kelvin := range vals {
				if hpKelvinTenths, ok := highPrecisionVals[instance]; ok {
					kelvin = hpKelvinTenths / 10.0
				}
				celsius := kelvin - kelvinOffset
				tags := []string{"thermal_zone:" + instance}
				sender.Gauge("system.thermal.temperature", celsius, "", tags)
			}
		}
	}

	// Collect passive limit values
	if c.passiveLimit != nil {
		vals, err := c.passiveLimit.GetAllValues()
		if err != nil {
			c.Warnf("thermal.Check: Error getting passive limit values: %v", err)
		} else {
			for instance, val := range vals {
				tags := []string{"thermal_zone:" + instance}
				sender.Gauge("system.thermal.passive_limit", val, "", tags)
			}
		}
	}

	sender.Commit()
	return nil
}
