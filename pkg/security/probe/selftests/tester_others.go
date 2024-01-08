// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package selftests

import (
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// SelfTester represents all the state needed to conduct rule injection test at startup
type SelfTester struct {
	probe *probe.Probe
}

// NewSelfTester returns a new SelfTester, enabled or not
func NewSelfTester(_ *config.RuntimeSecurityConfig, probe *probe.Probe) (*SelfTester, error) {
	return &SelfTester{
		probe: probe,
	}, nil
}

// RunSelfTest runs the self test and return the result
func (t *SelfTester) RunSelfTest() ([]string, []string, map[string]*serializers.EventSerializer, error) {
	return nil, nil, nil, nil
}

// Start starts the self tester policy provider
func (t *SelfTester) Start() {}

// Close removes temp directories and files used by the self tester
func (t *SelfTester) Close() error {
	return nil
}

// LoadPolicies implements the PolicyProvider interface
func (t *SelfTester) LoadPolicies(_ []rules.MacroFilter, _ []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	p := &rules.Policy{
		Name:       policyName,
		Source:     policySource,
		Version:    policyVersion,
		IsInternal: true,
	}

	return []*rules.Policy{p}, nil
}

// IsExpectedEvent sends an event to the tester
func (t *SelfTester) IsExpectedEvent(_ *rules.Rule, _ eval.Event, _ *probe.Probe) bool {
	return false
}
