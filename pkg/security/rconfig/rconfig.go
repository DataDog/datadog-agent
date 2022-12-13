// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package rconfig

import (
	"bytes"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const securityAgentRCPollInterval = time.Second * 1

// RCPolicyProvider defines a remote config policy provider
type RCPolicyProvider struct {
	sync.RWMutex

	client               *remote.Client
	onNewPoliciesReadyCb func()
	lastConfigs          map[string]state.ConfigCWSDD
}

var _ rules.PolicyProvider = (*RCPolicyProvider)(nil)

// NewRCPolicyProvider returns a new Remote Config based policy provider
func NewRCPolicyProvider(name string, agentVersion *semver.Version) (*RCPolicyProvider, error) {
	c, err := remote.NewGRPCClient(name, agentVersion.String(), []data.Product{data.ProductCWSDD}, securityAgentRCPollInterval)
	if err != nil {
		return nil, err
	}

	return &RCPolicyProvider{
		client: c,
	}, nil
}

// Start starts the Remote Config policy provider and subscribes to updates
func (r *RCPolicyProvider) Start() {
	log.Info("remote-config policies provider started")

	r.client.RegisterCWSDDUpdate(r.rcConfigUpdateCallback)

	r.client.Start()
}

func (r *RCPolicyProvider) rcConfigUpdateCallback(configs map[string]state.ConfigCWSDD) {
	r.Lock()
	r.lastConfigs = configs
	r.Unlock()

	log.Info("new policies from remote-config policy provider")

	r.onNewPoliciesReadyCb()
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

	for _, c := range r.lastConfigs {
		reader := bytes.NewReader(c.Config)

		policy, err := rules.LoadPolicy(c.Metadata.ID, "remote-config", reader, macroFilters, ruleFilters)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		normalize(policy)
		policies = append(policies, policy)
	}

	return policies, errs
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (r *RCPolicyProvider) SetOnNewPoliciesReadyCb(cb func()) {
	r.onNewPoliciesReadyCb = cb
}

// Close stops the client
func (r *RCPolicyProvider) Close() error {
	r.client.Close()
	return nil
}
