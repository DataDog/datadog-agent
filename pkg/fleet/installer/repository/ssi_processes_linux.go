// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package repository

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/module"
	"github.com/shirou/gopsutil/v3/process"
)

func getSsiProcess(p *process.Process) (InjectedProcess, bool, error) {
	injectionMeta, found := module.GetInjectionMeta(p)
	if !found {
		return InjectedProcess{}, false, nil
	}

	envs, err := module.GetEnvs(p, injectionMeta)
	if err != nil {
		return InjectedProcess{}, false, err
	}
	_, isInjected := envs["DD_INJECTION_ENABLED"]
	if !isInjected {
		return InjectedProcess{}, false, nil
	}

	serviceName, _ := envs["DD_SERVICE"]

	return InjectedProcess{
		Pid:            int(p.Pid),
		ServiceName:    emptyDefault(serviceName, "unknwown"),
		LanguageName:   injectionMeta.LanguageName,
		RuntimeVersion: injectionMeta.RuntimeVersion,

		LibraryVersion:  injectionMeta.TracerVersion,
		InjectorVersion: injectionMeta.InjectorVersion,
		IsInjected:      isInjected,
		InjectionStatus: emptyDefault(injectionMeta.InjectionStatus, "unknwown"),
		Reason:          injectionMeta.Reason,
	}, true, nil

}

func emptyDefault(s string, defaultStr string) string {
	if s == "" {
		return defaultStr
	}
	return s
}
