// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// RuleSetApplier defines a rule set applier. It applies rules using an Applier
type RuleSetApplier struct {
	config   *config.Config
	reporter *Reporter
}

// Applier describes the set of methods required to apply kernel event passing policies
type Applier interface {
	Init() error
	ApplyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag) error
	ApplyApprovers(eventType eval.EventType, approvers rules.Approvers) error
	RegisterKProbe(kprobe *ebpf.KProbe) error
	RegisterTracepoint(tracepoint string) error
}

func (rsa *RuleSetApplier) applyFilterPolicy(eventType eval.EventType, tableName string, mode PolicyMode, flags PolicyFlag, applier Applier) error {
	if err := rsa.reporter.SetFilterPolicy(eventType, tableName, mode, flags); err != nil {
		return err
	}

	if applier != nil {
		return applier.ApplyFilterPolicy(eventType, tableName, mode, flags)
	}

	return nil
}

func (rsa *RuleSetApplier) applyApprovers(eventType eval.EventType, approvers rules.Approvers, applier Applier) error {
	if err := rsa.reporter.SetApprovers(eventType, approvers); err != nil {
		return err
	}

	if applier != nil {
		return applier.ApplyApprovers(eventType, approvers)
	}

	return nil
}

func (rsa *RuleSetApplier) registerKProbe(kprobe *ebpf.KProbe, applier Applier) error {
	if applier != nil {
		return applier.RegisterKProbe(kprobe)
	}

	return nil
}

func (rsa *RuleSetApplier) registerTracepoint(tracepoint string, applier Applier) error {
	if applier != nil {
		return applier.RegisterTracepoint(tracepoint)
	}

	return nil
}

func (rsa *RuleSetApplier) setupKProbe(rs *rules.RuleSet, eventType eval.EventType, applier Applier) error {
	policyTable := allPolicyTables[eventType]
	if policyTable == "" {
		return nil
	}

	if !rsa.config.EnableKernelFilters {
		if err := rsa.applyFilterPolicy(eventType, policyTable, PolicyModeNoFilter, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	// if approvers disabled
	if !rsa.config.EnableApprovers {
		if err := rsa.applyFilterPolicy(eventType, policyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	capabilities, exists := allCapabilities[eventType]
	if !exists {
		return &ErrCapabilityNotFound{EventType: eventType}
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		if err := rsa.applyFilterPolicy(eventType, policyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyApprovers(eventType, approvers, applier); err != nil {
		if err := rsa.applyFilterPolicy(eventType, policyTable, PolicyModeAccept, math.MaxUint8, applier); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyFilterPolicy(eventType, policyTable, PolicyModeDeny, capabilities.GetFlags(), applier); err != nil {
		return err
	}

	return nil
}

// Apply applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (rsa *RuleSetApplier) Apply(rs *rules.RuleSet, applier Applier) (*Report, error) {
	alreadySetup := make(map[eval.EventType]bool)
	alreadyRegistered := make(map[*HookPoint]bool)

	if applier != nil {
		if err := applier.Init(); err != nil {
			return nil, err
		}
	}

	for _, hookPoint := range allHookPoints {
		if hookPoint.EventTypes == nil {
			continue
		}

		// first set policies
		for _, eventType := range hookPoint.EventTypes {
			if _, ok := alreadySetup[eventType]; ok {
				continue
			}

			if rs.HasRulesForEventType(eventType) {
				if err := rsa.setupKProbe(rs, eventType, applier); err != nil {
					return nil, err
				}
				alreadySetup[eventType] = true
			}
		}

		// then register kprobes
		for _, eventType := range hookPoint.EventTypes {
			if eventType == "*" || rs.HasRulesForEventType(eventType) {
				if _, ok := alreadyRegistered[hookPoint]; ok {
					continue
				}

				for _, kprobe := range hookPoint.KProbes {
					// use hook point name if kprobe name not provided
					if len(kprobe.Name) == 0 {
						kprobe.Name = hookPoint.Name
					}

					if err := rsa.registerKProbe(kprobe, applier); err != nil {
						if !hookPoint.Optional {
							return nil, err
						}
					}
				}

				if len(hookPoint.Tracepoint) > 0 {
					if err := rsa.registerTracepoint(hookPoint.Tracepoint, applier); err != nil {
						return nil, err
					}
				}
				alreadyRegistered[hookPoint] = true
			}
		}
	}

	return rsa.reporter.GetReport(), nil
}

// NewRuleSetApplier returns a new RuleSetApplier
func NewRuleSetApplier(cfg *config.Config) *RuleSetApplier {
	return &RuleSetApplier{
		config:   cfg,
		reporter: NewReporter(),
	}
}
