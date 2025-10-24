// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// GetSECLVariables returns the set of SECL variables along with theirs values
func (e *RuleEngine) GetSECLVariables() map[string]*api.SECLVariableState {
	rs := e.GetRuleSet()
	if rs == nil {
		return nil
	}

	preparator := e.newSECLVariableEventPreparator()

	rsVariables := rs.GetVariables()
	seclVariables := make(map[string]*api.SECLVariableState, len(rsVariables))

	e.fillCommonSECLVariables(rsVariables, seclVariables, preparator)

	for name, value := range rsVariables {
		if strings.HasPrefix(name, "container.") {
			scopedVariable := value.(eval.ScopedVariable)
			ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
			if !ok {
				continue
			}
			cgr := ebpfProbe.Resolvers.CGroupResolver

			cgr.Iterate(func(cgce *cgroupModel.CacheEntry) bool {
				cgce.RLock()
				defer cgce.RUnlock()
				if cgce.ContainerContext.ContainerID == "" {
					return false
				}

				ctx := preparator.get(func(event *model.Event) {
					event.ContainerContext = &cgce.ContainerContext
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx)
				if !found {
					return false
				}

				scopedName := fmt.Sprintf("%s.%s", name, cgce.ContainerContext.ContainerID)
				scopedValue := fmt.Sprintf("%v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
				return false
			})
		} else if !e.probe.Opts.EBPFLessEnabled && strings.HasPrefix(name, "cgroup.") {
			scopedVariable := value.(eval.ScopedVariable)

			cgr := e.probe.PlatformProbe.(*probe.EBPFProbe).Resolvers.CGroupResolver
			cgr.Iterate(func(cgce *cgroupModel.CacheEntry) bool {
				cgce.RLock()
				defer cgce.RUnlock()

				ctx := preparator.get(func(event *model.Event) {
					event.CGroupContext = &cgce.CGroupContext
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx)
				if !found {
					return false
				}

				scopedName := fmt.Sprintf("%s.%s", name, cgce.CGroupID)
				scopedValue := fmt.Sprintf("%v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
				return false
			})
		}
	}
	return seclVariables
}
