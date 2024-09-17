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

// AllowPolicy defines an allow policy strategy
type AllowPolicy struct {
	Action string   `yaml:"action,omitempty"`
	Allow  []string `yaml:"allow,omitempty"`
}

// SECLPolicy defines a SECL rule based policy
type SECLPolicy struct {
	Rules []*rules.RuleDefinition `yaml:"rules,omitempty"`
}

// WorkloadPolicy defines a workload policy
type WorkloadPolicy struct {
	ID   string
	Name string `yaml:"name"`
	Kind string `yaml:"kind"`

	AllowPolicy `yaml:",inline,omitempty"`
	SECLPolicy  `yaml:",inline,omitempty"`
}

// nolint: unused
func (p *SECLPolicy) convert(wp *WorkloadPolicy) ([]*rules.RuleDefinition, error) {
	// patch rules with the container ID
	for i, ruleDef := range wp.SECLPolicy.Rules {
		ruleDef.ID = fmt.Sprintf(`workload_policy_%s_%d`, wp.ID, i)
		ruleDef.GroupID = "workload_policy"
		ruleDef.Description = "Workload policy"
		ruleDef.Expression = fmt.Sprintf(`(%s) && container.id == "%s"`, ruleDef.Expression, wp.ID)
	}

	return wp.SECLPolicy.Rules, nil
}

// nolint: unused
func (p *AllowPolicy) convert(wp *WorkloadPolicy) ([]*rules.RuleDefinition, error) {
	var processes []string
	for _, allow := range p.Allow {
		processes = append(processes, fmt.Sprintf(`~"%s"`, allow))
	}

	// policy := &rules.Policy{
	// 	Name:   wp.Name,
	// 	Source: rules.PolicyProviderTypeWorkload,
	// }

	ruleDef := &rules.RuleDefinition{
		ID:          "workload_policy_" + wp.ID,
		GroupID:     "workload_policy",
		Description: "Workload policy",
		Expression:  fmt.Sprintf(`exec.file.path not in [%s] and container.id == "%s"`, strings.Join(processes, ","), wp.ID),
		// Policy:      policy,
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

	return []*rules.RuleDefinition{ruleDef}, nil
}

func (p *WorkloadPolicy) convert() (*rules.Policy, error) {
	var (
		// ruleDefs []*rules.RuleDefinition
		err error
	)

	// if p.Kind == "secl" {
	// 	ruleDefs, err = p.SECLPolicy.convert(p)
	// } else {
	// 	ruleDefs, err = p.AllowPolicy.convert(p)
	// }

	if err != nil {
		return nil, err
	}

	policy := &rules.Policy{
		Name:   p.Name,
		Source: rules.PolicyProviderTypeWorkload,
	}

	// for _, ruleDef := range ruleDefs {
	// 	ruleDef.Policy = policy
	// }

	// policy.Rules = ruleDefs

	return policy, nil
}

// WorkloadPolicyProvider defines a remote config policy provider
type WorkloadPolicyProvider struct {
	sync.RWMutex

	// internals
	debouncer            *debouncer.Debouncer
	onNewPoliciesReadyCb func()
	policies             map[*model.CacheEntry]WorkloadPolicy
}

// NewWorkloadPolicyProvider returns a new Remote Config based policy provider
func NewWorkloadPolicyProvider(resolver *cgroup.Resolver) (*WorkloadPolicyProvider, error) {
	wp := &WorkloadPolicyProvider{
		policies: make(map[*model.CacheEntry]WorkloadPolicy),
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

		var policy WorkloadPolicy
		if err = yaml.Unmarshal(file, &policy); err != nil {
			seclog.Errorf("unable to parse the policy file: %v", err)
		}

		//policy.ID = workload.ID

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
func (wp *WorkloadPolicyProvider) Start() {
	log.Info("workload policies provider started")
	wp.debouncer.Start()
}

// LoadPolicies implements the PolicyProvider interface
func (wp *WorkloadPolicyProvider) LoadPolicies(_ []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
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
