// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package rconfig

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/skydive-project/go-debouncer"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	securityAgentRCPollInterval = time.Second * 1
	debounceDelay               = 5 * time.Second
)

// RCPolicyProvider defines a remote config policy provider
type RCPolicyProvider struct {
	sync.RWMutex

	client               *remote.Client
	onNewPoliciesReadyCb func()
	lastDefaults         map[string]state.ConfigCWSDD
	lastCustoms          map[string]state.ConfigCWSCustom
	debouncer            *debouncer.Debouncer
}

var _ rules.PolicyProvider = (*RCPolicyProvider)(nil)

// NewRCPolicyProvider returns a new Remote Config based policy provider
func NewRCPolicyProvider() (*RCPolicyProvider, error) {
	agentVersion, err := utils.GetAgentSemverVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to parse agent version: %v", err)
	}

	c, err := remote.NewUnverifiedGRPCClient(agentName, agentVersion.String(), []data.Product{data.ProductCWSDD, data.ProductCWSCustom}, securityAgentRCPollInterval)
	if err != nil {
		return nil, err
	}

	r := &RCPolicyProvider{
		client: c,
	}
	r.debouncer = debouncer.New(debounceDelay, r.onNewPoliciesReady)

	return r, nil
}

// Start starts the Remote Config policy provider and subscribes to updates
func (r *RCPolicyProvider) Start() {
	log.Info("remote-config policies provider started")

	r.debouncer.Start()

	r.client.RegisterCWSDDUpdate(r.rcDefaultsUpdateCallback)
	r.client.RegisterCWSCustomUpdate(r.rcCustomsUpdateCallback)

	r.client.Start()
}

func (r *RCPolicyProvider) rcDefaultsUpdateCallback(configs map[string]state.ConfigCWSDD) {
	r.Lock()
	r.lastDefaults = configs
	r.Unlock()

	log.Info("new policies from remote-config policy provider")

	r.debouncer.Call()
}

func (r *RCPolicyProvider) rcCustomsUpdateCallback(configs map[string]state.ConfigCWSCustom) {
	r.Lock()
	r.lastCustoms = configs
	r.Unlock()

	log.Info("new policies from remote-config policy provider")

	r.debouncer.Call()
}

func normalize(policy *rules.Policy) {
	// remove the version
	_, normalized, found := strings.Cut(policy.Name, ".")
	if found {
		policy.Name = normalized
	}
}

// LoadPolicies implements the PolicyProvider interface
func (r *RCPolicyProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	var policies []*rules.Policy
	var errs *multierror.Error

	r.RLock()
	defer r.RUnlock()

	load := func(id string, cfg []byte) {
		reader := bytes.NewReader(cfg)

		policy, err := rules.LoadPolicy(id, "remote-config", reader, macroFilters, ruleFilters)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			normalize(policy)
			policies = append(policies, policy)
		}
	}

	for _, c := range r.lastDefaults {
		load(c.Metadata.ID, c.Config)
	}
	for _, c := range r.lastCustoms {
		load(c.Metadata.ID, c.Config)
	}

	return policies, errs
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (r *RCPolicyProvider) SetOnNewPoliciesReadyCb(cb func()) {
	r.onNewPoliciesReadyCb = cb
}

func (r *RCPolicyProvider) onNewPoliciesReady() {
	r.RLock()
	defer r.RUnlock()

	if r.onNewPoliciesReadyCb != nil {
		r.onNewPoliciesReadyCb()
	}
}

// Close stops the client
func (r *RCPolicyProvider) Close() error {
	r.debouncer.Stop()
	r.client.Close()
	return nil
}
