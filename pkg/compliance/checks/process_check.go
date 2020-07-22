// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidity time.Duration = 10 * time.Minute

	processFieldName    = "process.name"
	processFieldExe     = "process.exe"
	processFieldCmdLine = "process.cmdLine"
	processFuncFlag     = "process.flag"
	processFuncHasFlag  = "process.hasFlag"
)

var processReportedFields = []string{
	processFieldName,
	processFieldExe,
	processFieldCmdLine,
}

func checkProcess(e env.Env, id string, res compliance.Resource, expr *eval.IterableExpression) (*report, error) {
	if res.Process == nil {
		return nil, fmt.Errorf("%s: expecting process resource in process check", id)
	}

	process := res.Process

	log.Debugf("%s: running process check: %s", id, process.Name)

	processes, err := getProcesses(cacheValidity)

	if err != nil {
		return nil, log.Errorf("%s: Unable to fetch processes: %v", id, err)
	}

	matchedProcesses := processes.findProcessesByName(process.Name)

	var instances []*eval.Instance
	for _, mp := range matchedProcesses {

		flagValues := parseProcessCmdLine(mp.Cmdline)
		instance := &eval.Instance{
			Vars: eval.VarMap{
				processFieldName:    mp.Name,
				processFieldExe:     mp.Exe,
				processFieldCmdLine: mp.Cmdline,
			},
			Functions: eval.FunctionMap{
				processFuncFlag:    processFlag(flagValues),
				processFuncHasFlag: processHasFlag(flagValues),
			},
		}
		instances = append(instances, instance)
	}

	it := &instanceIterator{
		instances: instances,
	}

	result, err := expr.EvaluateIterator(it, globalInstance)
	if err != nil {
		return nil, err
	}

	return instanceResultToReport(result, processReportedFields), nil
}

func processFlag(flagValues map[string]string) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		flag, err := validateProcessFlagArg(args...)
		if err != nil {
			return nil, err
		}
		value, _ := flagValues[flag]
		return value, nil
	}
}
func processHasFlag(flagValues map[string]string) eval.Function {
	return func(_ *eval.Instance, args ...interface{}) (interface{}, error) {
		flag, err := validateProcessFlagArg(args...)
		if err != nil {
			return nil, err
		}
		_, has := flagValues[flag]
		return has, nil
	}
}

func validateProcessFlagArg(args ...interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf(`invalid number of arguments, expecting 1 got %d`, len(args))
	}
	flag, ok := args[0].(string)
	if !ok {
		return "", errors.New(`expecting string value for flag argument`)
	}
	return flag, nil
}
