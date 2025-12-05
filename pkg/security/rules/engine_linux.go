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

			ebpfProbe.Walk(func(entry *model.ProcessCacheEntry) {
				entry.Retain()
				defer entry.Release()

				if entry.ContainerContext.ContainerID == "" {
					return
				}

				ctx := preparator.get(func(event *model.Event) {
					event.ProcessCacheEntry = entry
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx)
				if !found {
					return
				}

				scopedName := fmt.Sprintf("%s.%s", name, entry.ContainerContext.ContainerID)
				scopedValue := fmt.Sprintf("%v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
			})
		} else if !e.probe.Opts.EBPFLessEnabled && strings.HasPrefix(name, "cgroup.") {
			scopedVariable := value.(eval.ScopedVariable)
			ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
			if !ok {
				continue
			}

			ebpfProbe.Walk(func(entry *model.ProcessCacheEntry) {
				entry.Retain()
				defer entry.Release()

				if entry.ProcessContext.Process.CGroup.CGroupFile.IsNull() {
					return
				}

				ctx := preparator.get(func(event *model.Event) {
					event.ProcessCacheEntry = entry
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx)
				if !found {
					return
				}

				scopedName := fmt.Sprintf("%s.%s", name, entry.ProcessContext.CGroup.CGroupID)
				scopedValue := fmt.Sprintf("%v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
			})
		}
	}
	return seclVariables
}
