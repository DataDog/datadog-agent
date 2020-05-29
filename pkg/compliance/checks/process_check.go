// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

const (
	cacheValidity time.Duration = 10 * time.Minute
)

type processCheck struct {
	baseCheck
	process *compliance.Process
}

func newProcessCheck(baseCheck baseCheck, process *compliance.Process) (*processCheck, error) {
	if len(process.Name) == 0 {
		return nil, fmt.Errorf("Unable to create processCheck without a process name")
	}

	return &processCheck{
		baseCheck: baseCheck,
		process:   process,
	}, nil
}

func (c *processCheck) Run() error {
	log.Debugf("%s: process check: %s", c.ruleID, c.process.Name)
	processes, err := getProcesses(cacheValidity)
	if err != nil {
		return log.Errorf("Unable to fetch processes: %v", err)
	}

	matchedProcesses := processes.findProcessesByName(c.process.Name)
	for _, mp := range matchedProcesses {
		err = c.reportProcess(mp)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *processCheck) reportProcess(p *process.FilledProcess) error {
	log.Debugf("%s: process check - match %s", c.ruleID, p.Cmdline)
	kv := compliance.KV{}
	flagValues := parseProcessCmdLine(p.Cmdline)

	for _, field := range c.process.Report {
		switch field.Kind {
		case "flag":
			if flagValue, found := flagValues[field.Property]; found {
				flagReportName := field.Property
				if len(field.As) > 0 {
					flagReportName = field.As
				}
				if len(field.Value) > 0 {
					flagValue = field.Value
				}

				kv[flagReportName] = flagValue
			}
		default:
			return log.Errorf("Unsupported kind value: '%s' for process: '%s'", field.Kind, p.Name)
		}
	}

	c.report(nil, kv)
	return nil
}
