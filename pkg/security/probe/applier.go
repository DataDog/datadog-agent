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

func (rsa *RuleSetApplier) setupFilters(rs *rules.RuleSet, eventType eval.EventType) error {
	if !rsa.config.EnableKernelFilters {
		if err := rsa.applyFilterPolicy(eventType, PolicyModeNoFilter, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	// if approvers disabled
	if !rsa.config.EnableApprovers {
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	capabilities, exists := allCapabilities[eventType]
	if !exists {
		return rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8)
	}

	approvers, err := rs.GetApprovers(eventType, capabilities.GetFieldCapabilities())
	if err != nil {
		if err := rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyApprovers(eventType, approvers); err != nil {
		log.Errorf("Failed to apply approvers, setting policy mode to 'accept' (error: %s)", err)
		if err := rsa.applyFilterPolicy(eventType, PolicyModeAccept, math.MaxUint8); err != nil {
			return err
		}
		return nil
	}

	if err := rsa.applyFilterPolicy(eventType, PolicyModeDeny, capabilities.GetFlags()); err != nil {
		return err
	}

	return nil
}

// Apply setup the filters for the provided set of rules and returns the policy report.
func (rsa *RuleSetApplier) Apply(rs *rules.RuleSet) (*Report, error) {
	if rsa.probe != nil {
		if err := rsa.probe.FlushDiscarders(); err != nil {
			return nil, errors.Wrap(err, "failed to flush discarders")
		}

		// based on the ruleset and the requested rules, select the probes that need to be activated
		if err := rsa.probe.SelectProbes(rs); err != nil {
			return nil, errors.Wrap(err, "failed to select probes")
		}
	}

	for _, eventType := range rs.GetEventTypes() {
		if err := rsa.setupFilters(rs, eventType); err != nil {
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
