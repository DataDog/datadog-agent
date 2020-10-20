// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"math"

	"github.com/DataDog/ebpf/manager"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	RegisterProbesSelectors(selectors []manager.ProbesSelector) error
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

func (rsa *RuleSetApplier) registerProbesSelectors(selectors []manager.ProbesSelector, applier Applier) error {
	if applier != nil {
		return applier.RegisterProbesSelectors(selectors)
	}
	return nil
}

func (rsa *RuleSetApplier) setupFilters(rs *rules.RuleSet, eventType eval.EventType, applier Applier) error {
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

// Apply setup the filters for the provided set of rules and returns the policy report.
func (rsa *RuleSetApplier) Apply(rs *rules.RuleSet, applier Applier) (*Report, error) {
	for eventType := range probes.SelectorsPerEventType {
		if rs.HasRulesForEventType(eventType) {
			if err := rsa.setupFilters(rs, eventType, applier); err != nil {
				return nil, err
			}
		}
	}
	return rsa.reporter.GetReport(), nil
}

// SelectProbes applies the loaded set of rules and returns a report
// of the applied approvers for it.
func (rsa *RuleSetApplier) SelectProbes(rs *rules.RuleSet, applier Applier) error {
	var selectedIDs []manager.ProbeIdentificationPair
	for eventType, selectors := range probes.SelectorsPerEventType {
		if eventType == "*" || rs.HasRulesForEventType(eventType) {
			// register probes selectors
			if err := rsa.registerProbesSelectors(selectors, applier); err != nil {
				return err
			}

			// Extract unique IDs for logging purposes only
			for _, selector := range selectors {
				for _, id := range selector.GetProbesIdentificationPairList() {
					var exists bool
					for _, selectedID := range selectedIDs {
						if selectedID.Matches(id) {
							exists = true
						}
					}
					if !exists {
						selectedIDs = append(selectedIDs, id)
					}
				}
			}
		}
	}

	// Print the list of unique probe identification IDs that are registered
	for _, id := range selectedIDs {
		log.Debugf("probe %s registered", id)
	}
	return nil
}

// NewRuleSetApplier returns a new RuleSetApplier
func NewRuleSetApplier(cfg *config.Config) *RuleSetApplier {
	return &RuleSetApplier{
		config:   cfg,
		reporter: NewReporter(),
	}
}
