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
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
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
				if entry.ContainerContext.IsNull() {
					return
				}

				ctx := preparator.get(func(event *model.Event) {
					event.ProcessCacheEntry = entry
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx, true) // for status, let's not follow inheritance
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
				if entry.ProcessContext.Process.CGroup.IsNull() {
					return
				}

				ctx := preparator.get(func(event *model.Event) {
					event.ProcessCacheEntry = entry
				})
				defer preparator.put(ctx)

				value, found := scopedVariable.GetValue(ctx, true) // for status, let's not follow inheritance
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

// ConnectSBOMResolver arranges for the rule set to be reloaded once the core
// agent has said which workloads it scans.
//
// A rule reading a package field is admitted while the answer is outstanding,
// since rejecting it on a guess would break a valid rule with no way back.
// Reloading when the answer arrives settles the status either way.
func (e *RuleEngine) ConnectSBOMResolver() {
	ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok || ebpfProbe.Resolvers == nil || ebpfProbe.Resolvers.SBOMResolver == nil {
		return
	}

	ebpfProbe.Resolvers.SBOMResolver.SetCapabilitiesCallback(func() {
		if err := e.ReloadPolicies(false); err != nil {
			seclog.Warnf("could not reload policies after the SBOM capabilities arrived: %v", err)
		}
	})
}

// unavailableFieldPrefixes names the field prefixes whose source is not running.
// The package fields are read from the file index the core agent publishes for
// each workload it scans, so where it scans nothing there is nothing to read
// them from, and a rule naming one is better rejected than left to match
// nothing in silence.
//
// An unanswered handshake is not an answer: until the core agent replies the
// fields are treated as available, and the reply triggers the reload that
// settles the status.
func (e *RuleEngine) unavailableFieldPrefixes() []string {
	ebpfProbe, ok := e.probe.PlatformProbe.(*probe.EBPFProbe)
	if !ok || ebpfProbe.Resolvers == nil || ebpfProbe.Resolvers.SBOMResolver == nil {
		return nil
	}
	if ebpfProbe.Resolvers.SBOMResolver.LacksWorkloadIndexes() {
		return []string{".package."}
	}
	return nil
}
