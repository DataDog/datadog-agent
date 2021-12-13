// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"math"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/pkg/errors"
)

// RuleSetApplier defines a rule set applier. It applies rules using an Applier
type RuleSetApplier struct {
	config   *config.Config
	reporter *Reporter
	probe    *Probe
}

func (rsa *RuleSetApplier) applyFilterPolicy(eventType eval.EventType, mode PolicyMode, flags PolicyFlag) error {
	if err := rsa.reporter.SetFilterPolicy(eventType, mode, flags); err != nil {
		return err
	}

	if rsa.probe != nil {
		return rsa.probe.ApplyFilterPolicy(eventType, mode, flags)
	}

	return nil
}

func (rsa *RuleSetApplier) applyApprovers(eventType eval.EventType, approvers rules.Approvers) error {
	if err := rsa.reporter.SetApprovers(eventType, approvers); err != nil {
		return err
	}

	if rsa.probe != nil {
		if err := rsa.probe.SetApprovers(eventType, approvers); err != nil {
			return err
		}
	}

	return nil
}

// applyDefaultPolicy this will apply the deny policy if kernel filters are enabled
func (rsa *RuleSetApplier) applyDefaultFilterPolicies() {
	var model Model
	for _, eventType := range model.GetEventTypes() {
		if !rsa.config.EnableKernelFilters {
			_ = rsa.applyFilterPolicy(eventType, PolicyModeNoFilter, math.MaxUint8)
		} else {
			_ = rsa.applyFilterPolicy(eventType, PolicyModeDeny, math.MaxUint8)
		}
	}
}

func (rsa *RuleSetApplier) setupFilters(rs *rules.RuleSet, eventType eval.EventType, approvers rules.Approvers) error {
	if !rsa.config.EnableKernelFilters {
		return rsa.applyFilterPolicy(eventType, PolicyModeNoFilter, math.MaxUint8)
	}

	// if approvers disabled
	if !rsa.config.EnableApprovers {
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	capabilities, exists := allCapabilities[eventType]
	if !exists {
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	if len(approvers) == 0 {
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	if err := rsa.applyApprovers(eventType, approvers); err != nil {
		log.Errorf("Failed to apply approvers, setting policy mode to 'accept' (error: %s)", err)
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	return rsa.applyFilterPolicy(eventType, PolicyModeDeny, capabilities.GetFlags())
}

// Apply setup the filters for the provided set of rules and returns the policy report.
func (rsa *RuleSetApplier) Apply(rs *rules.RuleSet, approvers map[eval.EventType]rules.Approvers) (*Report, error) {
	if rsa.probe != nil {
		// based on the ruleset and the requested rules, select the probes that need to be activated
		if err := rsa.probe.SelectProbes(rs); err != nil {
			return nil, errors.Wrap(err, "failed to select probes")
		}

		if err := rsa.probe.OnRuleSetApplied(); err != nil {
			return nil, err
		}
	}

	// apply deny filter by default
	rsa.applyDefaultFilterPolicies()

	for _, eventType := range rs.GetEventTypes() {
		if err := rsa.setupFilters(rs, eventType, approvers[eventType]); err != nil {
			return nil, err
		}
	}

	return rsa.reporter.GetReport(), nil
}

// NewRuleSetApplier returns a new RuleSetApplier
func NewRuleSetApplier(cfg *config.Config, probe *Probe) *RuleSetApplier {
	return &RuleSetApplier{
		config:   cfg,
		probe:    probe,
		reporter: NewReporter(),
	}
}
