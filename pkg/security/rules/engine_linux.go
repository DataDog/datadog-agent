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
)

// GetSECLVariables returns the set of SECL variables along with theirs values
func (e *RuleEngine) GetSECLVariables() map[string]*api.SECLVariableState {
	rs := e.GetRuleSet()
	if rs == nil {
		return nil
	}

	seclVariables := e.getCommonSECLVariables(rs)
	for name, value := range rs.GetVariables() {
		if strings.HasPrefix(name, "container.") {
			scopedVariable := value.(eval.ScopedVariable)
			ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
			if !ok {
				continue
			}
			cgr := ebpfProbe.Resolvers.CGroupResolver
			containerWorkloads := cgr.GetContainerWorkloads()
			if containerWorkloads == nil {
				continue
			}

			for _, cgce := range containerWorkloads.Values() {
				cgce.RLock()
				defer cgce.RUnlock()

				event := e.probe.PlatformProbe.NewEvent()
				event.ContainerContext = &cgce.ContainerContext
				ctx := eval.NewContext(event)
				scopedName := fmt.Sprintf("%s.%s", name, cgce.ContainerContext.ContainerID)
				value, found := scopedVariable.GetValue(ctx)
				if !found {
					continue
				}

				scopedValue := fmt.Sprintf("%v", value)
				seclVariables[scopedName] = &api.SECLVariableState{
					Name:  scopedName,
					Value: scopedValue,
				}
			}
		}
	}
	return seclVariables
}
