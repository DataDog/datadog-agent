// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Portions of this code are taken from the gopsutil project
// https://github.com/shirou/gopsutil .  This code is licensed under the New BSD License
// copyright WAKAYAMA Shirou, and the gopsutil contributors

//go:build windows

package cpu

import (
	"fmt"
	"strconv"

	"github.com/DataDog/gohai/cpu"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/pdhutil"
)

const cpuCheckName = "cpu"

// For testing purposes
var cpuInfo = cpu.GetCpuInfo

// Check doesn't need additional fields
type Check struct {
	core.CheckBase
	nbCPU    float64
	pdhQuery *pdhutil.PdhQuery
	// maps metric to counter object
	counters map[string]pdhutil.PdhSingleInstanceCounter
}

// Run executes the check
func (c *Check) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	sender.Gauge("system.cpu.num_cores", c.nbCPU, "", nil)

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
				c.Warnf("cpu.Check: Could not retrieve value for %v: %v", metricname, err)
			}
		}
	} else {
		c.Warnf("cpu.Check: Could not collect performance counter data: %v", err)
	}

	sender.Gauge("system.cpu.iowait", 0.0, "", nil)
	sender.Gauge("system.cpu.stolen", 0.0, "", nil)
	sender.Gauge("system.cpu.guest", 0.0, "", nil)
	sender.Commit()

	return nil
}

// Overriding AddToQuery, see that function for details
type processorPDHCounter struct {
	pdhutil.PdhEnglishSingleInstanceCounter
}

func (counter *processorPDHCounter) addToQueryWithExtraChecks(query *pdhutil.PdhQuery, objectName string) (err error) {
	// addToQueryWithExtraChecks wraps the base counter's AddToQuery to add additional error checks
	// to work around a possible Microsoft PDH issue.
	//
	// The "Processor Information" counterset does not work in Windows containers.
	// The counterset exists but has no instances and always returns "PDH_NO_DATA" error.
	// So we have to fallback to the old "Processor" counterset.
	// see https://github.com/DataDog/datadog-agent/pull/8881
	// Unfortunately, since the counterset exists checking the PdhAddEnglishCounter result
	// alone is insufficient. We must perform additional checks to ensure the counterset
	// is functioning properly.

	// Add counter to the Query
	var origObjectName = counter.ObjectName
	counter.ObjectName = objectName
	err = counter.PdhEnglishSingleInstanceCounter.AddToQuery(query)
	if err != nil {
		// PdhAddEnglishCounter failed, restore original object name and return
		counter.ObjectName = origObjectName
		return err
	}
	defer func() {
		if err != nil {
			// failed, restore original object name
			counter.ObjectName = origObjectName
			// Remove the counter from the query
			tmpErr := counter.Remove()
			if tmpErr != nil {
				log.Warnf("cpu.Check: Failed to remove counter \\%s(%s)\\%s from query. %v", objectName, counter.InstanceName, counter.CounterName, tmpErr)
			}
		}
	}()

	// Add succeeded, check if the counter is working (see above comment)
	// Must call PdhCollectQueryData() twice before GetValue() will succeed.
	// Ignoring PdhCollectQueryData() return value because its success is determined by the query
	// and not specifically this counter, additionally if either call fails then GetValue() will too.
	_ = pdhutil.PdhCollectQueryData(query.Handle)
	_ = pdhutil.PdhCollectQueryData(query.Handle)
	_, err = counter.GetValue()
	return err
}

func (counter *processorPDHCounter) AddToQuery(query *pdhutil.PdhQuery) error {
	// Configure the PDH counter according to the running environment.
	// This check defaults to using the "Processor Information" counterset when it is available,
	// as it is the newer version of the "Processor" counterset and has support for more features.
	// https://techcommunity.microsoft.com/t5/running-sap-applications-on-the/windows-2008-r2-performance-monitor-8211-processor-information/ba-p/367007
	// See addToQueryWithExtraChecks for more details.
	err := counter.addToQueryWithExtraChecks(query, "Processor Information")
	if err != nil {
		log.Warnf("cpu.Check: Error initializing counter \\%s(%s)\\%s: %v. This error is expected in Windows containers. Trying Processor counterset as a fallback.", counter.ObjectName, counter.InstanceName, counter.CounterName, err)
		err = counter.addToQueryWithExtraChecks(query, "Processor")
	}

	return err
}

func addProcessorPdhCounter(query *pdhutil.PdhQuery, counterName string) pdhutil.PdhSingleInstanceCounter {
	var counter processorPDHCounter
	counter.Initialize("Processor Information", counterName, "_Total")
	query.AddCounter(&counter)
	return &counter
}

// Configure the CPU check doesn't need configuration
func (c *Check) Configure(integrationConfigDigest uint64, data integration.Data, initConfig integration.Data, source string) error {
	if err := c.CommonConfigure(integrationConfigDigest, initConfig, data, source); err != nil {
		return err
	}

	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("cpu.Check: could not query CPU info")
	}
	cpucount, _ := strconv.ParseFloat(info["cpu_logical_processors"], 64)
	c.nbCPU = cpucount

	// Create PDH query
	c.pdhQuery, err = pdhutil.CreatePdhQuery()
	if err != nil {
		return err
	}

	c.counters = map[string]pdhutil.PdhSingleInstanceCounter{
		"system.cpu.interrupt": addProcessorPdhCounter(c.pdhQuery, "% Interrupt Time"),
		"system.cpu.idle":      addProcessorPdhCounter(c.pdhQuery, "% Idle Time"),
		"system.cpu.user":      addProcessorPdhCounter(c.pdhQuery, "% User Time"),
		"system.cpu.system":    addProcessorPdhCounter(c.pdhQuery, "% Privileged Time"),
	}

	return nil
}

func cpuFactory() check.Check {
	return &Check{
		CheckBase: core.NewCheckBase(cpuCheckName),
	}
}

func init() {
	core.RegisterCheck(cpuCheckName, cpuFactory)
}
