// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	processutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// CacheValidity holds the default validity duration for a process in the cache
	CacheValidity time.Duration = 10 * time.Minute
)

var reportedFields = []string{
	compliance.ProcessFieldName,
	compliance.ProcessFieldCmdLine,
}

func resolve(_ context.Context, e env.Env, id string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	if res.Process == nil {
		return nil, fmt.Errorf("%s: expecting process resource in process check", id)
	}

	matchedProcesses, err := processutils.FindProcessesByName(res.Process.Name)
	if err != nil {
		return nil, log.Errorf("%s: Unable to fetch processes: %v", id, err)
	}

	var instances []resources.ResolvedInstance
	for _, mp := range matchedProcesses {
		name := mp.Name
		cmdLine := mp.Cmdline
		flagValues := mp.CmdlineFlags()
		envs := mp.EnvsMap(res.Process.Envs)

		instance := eval.NewInstance(
			eval.VarMap{
				compliance.ProcessFieldName:    name,
				compliance.ProcessFieldCmdLine: cmdLine,
				compliance.ProcessFieldFlags:   flagValues,
			},
			eval.FunctionMap{
				compliance.ProcessFuncFlag:    processFlag(flagValues),
				compliance.ProcessFuncHasFlag: processHasFlag(flagValues),
			},
			eval.RegoInputMap{
				"name":    name,
				"exe":     "", // NOTE(pierre): this field will be removed in next release. Only kept for compat reasons.
				"cmdLine": cmdLine,
				"flags":   flagValues,
				"pid":     mp.Pid,
				"envs":    envs,
			},
		)
		instances = append(instances, resources.NewResolvedInstance(instance, strconv.Itoa(int(mp.Pid)), "process"))
	}

	if len(instances) == 0 && rego {
		return nil, nil
	}

	return resources.NewResolvedInstances(instances), nil
}

func processFlag(flagValues map[string]string) eval.Function {
	return func(_ eval.Instance, args ...interface{}) (interface{}, error) {
		flag, err := validateProcessFlagArg(args...)
		if err != nil {
			return nil, err
		}
		value := flagValues[flag]
		return value, nil
	}
}
func processHasFlag(flagValues map[string]string) eval.Function {
	return func(_ eval.Instance, args ...interface{}) (interface{}, error) {
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

func init() {
	resources.RegisterHandler("process", resolve, reportedFields)
}
