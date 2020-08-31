// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type RuleSetApplier struct {
	config   *config.Config
	reporter *Reporter
}

// Applier describes the set of methods required to apply kernel event passing policies
type Applier interface {
	Init() error
	ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error
	ApplyApprovers(eventType eval.EventType, hook *HookPoint, approvers rules.Approvers) error
	RegisterKProbe(kprobe *KProbe) error
	RegisterTracepoint(tracepoint string) error
}

func (rsa *RuleSetApplier) applyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag, applier Applier) error {
	if err := rsa.reporter.ApplyFilterPolicy(eventType, tableName, mode, flags); err != nil {
		return err
	}

	if applier != nil {
		return applier.ApplyFilterPolicy(eventType, tableName, mode, flags)
	}

	return nil
}

func (rsa *RuleSetApplier) applyApprovers(eventType eval.EventType, hook *HookPoint, approvers rules.Approvers, applier Applier) error {
	if err := rsa.reporter.ApplyApprovers(eventType, hook, approvers); err != nil {
		return err
	}

	if applier != nil {
		return applier.ApplyApprovers(eventType, hook, approvers)
	}

	return nil
}

func (rsa *RuleSetApplier) RegisterKProbe(kprobe *KProbe, applier Applier) error {
	if applier != nil {
		return applier.RegisterKProbe(kprobe)
	}

	return nil
}

func (rsa *RuleSetApplier) RegisterTracepoint(tracepoint string, applier Applier) error {
	if applier != nil {
		return applier.RegisterTracepoint(tracepoint)
	}

	return nil
}

func (rsa *RuleSetApplier) setKProbePolicy(hookPoint *HookPoint, rs *rules.RuleSet, eventType eval.EventType, capabilities Capabilities, applier Applier) error {
	if !rsa.config.EnableKernelFilters {
		if err := rsa.applyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeNoFilter, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	// if approvers disabled
	if !rsa.config.EnableApprovers {
		if err := rsa.applyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		if err := rsa.applyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyApprovers(eventType, hookPoint, approvers, applier); err != nil {
		if err := rsa.applyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyFilterPolicy(eventType, hookPoint.PolicyTable, PolicyModeDeny, capabilities.GetFlags(), applier); err != nil {
		return err
	}

	return nil
}

// Apply applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (rsa *RuleSetApplier) Apply(rs *rules.RuleSet, applier Applier) (*Report, error) {
	already := make(map[*HookPoint]bool)

	if err := applier.Init(); err != nil {
		return nil, err
	}

	for _, hookPoint := range allHookPoints {
		if hookPoint.EventTypes == nil {
			continue
		}

		// first set policies
		for _, eventType := range hookPoint.EventTypes {
			if rs.HasRulesForEventType(eventType) {
				if hookPoint.PolicyTable == "" {
					continue
				}

				capabilities := allCapabilities[eventType]

				if err := rsa.setKProbePolicy(hookPoint, rs, eventType, capabilities, applier); err != nil {
					return nil, err
				}
			}
		}

		// then register kprobes
		for _, eventType := range hookPoint.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
				if _, ok := already[hookPoint]; ok {
					continue
				}

				for _, kprobe := range hookPoint.KProbes {
					// use hook point name if kprobe name not provided
					if len(kprobe.Name) == 0 {
						kprobe.Name = hookPoint.Name
					}

					if err := rsa.RegisterKProbe(kprobe, applier); err != nil {
						if !hookPoint.Optional {
							return nil, err
						}
					}
				}

				if len(hookPoint.Tracepoint) > 0 {
					if err := rsa.RegisterTracepoint(hookPoint.Tracepoint, applier); err != nil {
						return nil, err
					}
				}
				already[hookPoint] = true
			}
		}
	}

	return rsa.reporter.GetReport(), nil
}

func NewRuleSetApplier(cfg *config.Config) *RuleSetApplier {
	return &RuleSetApplier{
		config:   cfg,
		reporter: NewReporter(),
	}
}
