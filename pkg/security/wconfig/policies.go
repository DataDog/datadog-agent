// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package wconfig holds rconfig related files
package wconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/skydive-project/go-debouncer"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	debounceDelay = time.Second
)

type Policy struct {
	ID     string
	Name   string   `yaml:"name"`
	Action string   `yaml:"action"`
	Allow  []string `yaml:"allow"`
}

func (p *Policy) convert() (*rules.Policy, error) {
	var processes []string
	for _, allow := range p.Allow {
		processes = append(processes, fmt.Sprintf(`~"%s"`, allow))
	}

	policy := &rules.Policy{
		Name:   p.Name,
		Source: rules.PolicyProviderTypeWorkload,
	}

	ruleDef := &rules.RuleDefinition{
		ID:          "workload_policy_" + p.ID,
		GroupID:     "workload_policy",
		Description: "Workload policy",
		Expression:  fmt.Sprintf(`exec.file.path not in [%s] and container.id == "%s"`, strings.Join(processes, ","), p.ID),
		Policy:      policy,
	}

	if p.Action == "kill" {
		ruleDef.Actions = []*rules.ActionDefinition{
			{
				Kill: &rules.KillDefinition{
					Signal: "SIGKILL",
					Scope:  "container",
				},
			},
		}
	}

	policy.Rules = append(policy.Rules, ruleDef)

	return policy, nil
}

// WorkloadPolicyProvider defines a remote config policy provider
type WorkloadPolicyProvider struct {
	sync.RWMutex

	// internals
	debouncer            *debouncer.Debouncer
	onNewPoliciesReadyCb func()
	policies             map[*model.CacheEntry]Policy
}

// NewWorkloadPolicyProvider returns a new Remote Config based policy provider
func NewWorkloadPolicyProvider(resolver *cgroup.Resolver) (*WorkloadPolicyProvider, error) {
	wp := &WorkloadPolicyProvider{
		policies: make(map[*model.CacheEntry]Policy),
	}
	wp.debouncer = debouncer.New(debounceDelay, wp.onNewPoliciesReady)

	if err := resolver.RegisterListener(cgroup.CGroupCreated, wp.onCGroupCreatedEvent); err != nil {
		return nil, err
	}
	if err := resolver.RegisterListener(cgroup.CGroupDeleted, wp.onCGroupDeletedEvent); err != nil {
		return nil, err
	}

	return wp, nil
}

func (wp *WorkloadPolicyProvider) onCGroupCreatedEvent(workload *model.CacheEntry) {
	wp.Lock()
	defer wp.Unlock()

	if _, exists := wp.policies[workload]; exists {
		return
	}

	for pid := range workload.PIDs {
		policyPath := filepath.Join(kernel.ProcFSRoot(), fmt.Sprintf("/%d/root/.cws-policy.yaml", pid))
		file, err := os.ReadFile(policyPath)
		if err != nil {
			continue
		}

		var policy Policy
		if err = yaml.Unmarshal(file, &policy); err != nil {
			seclog.Errorf("unable to parse the policy file: %v", err)
		}
		policy.ID = workload.ID

		wp.policies[workload] = policy

		wp.debouncer.Call()

		return
	}
}

func (wp *WorkloadPolicyProvider) onCGroupDeletedEvent(workload *model.CacheEntry) {
	wp.Lock()
	defer wp.Unlock()

	delete(wp.policies, workload)

	wp.debouncer.Call()
}

// Start starts the Remote Config policy provider and subscribes to updates
func (r *WorkloadPolicyProvider) Start() {
	log.Info("workload policies provider started")
	r.debouncer.Start()
}

// LoadPolicies implements the PolicyProvider interface
func (wp *WorkloadPolicyProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	var policies []*rules.Policy
	var errs *multierror.Error

	wp.RLock()
	defer wp.RUnlock()

	for _, policy := range wp.policies {
		p, err := policy.convert()
		if err != nil {
			errs = multierror.Append(errs, err)
			continue
		}

		policies = append(policies, p)
	}

	return policies, errs
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (wp *WorkloadPolicyProvider) SetOnNewPoliciesReadyCb(cb func()) {
	wp.onNewPoliciesReadyCb = cb
}

func (wp *WorkloadPolicyProvider) onNewPoliciesReady() {
	wp.RLock()
	defer wp.RUnlock()

	if wp.onNewPoliciesReadyCb != nil {
		wp.onNewPoliciesReadyCb()
	}
}

// Close stops the client
func (wp *WorkloadPolicyProvider) Close() error {
	return nil
}

// Type returns the type of this policy provider
func (wp *WorkloadPolicyProvider) Type() string {
	return rules.PolicyProviderTypeWorkload
}
