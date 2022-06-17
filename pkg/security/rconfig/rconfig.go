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

	"github.com/DataDog/datadog-agent/pkg/config/remote"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/hashicorp/go-multierror"
)

// RCPolicyProvider defines a remote config policy provider
type RCPolicyProvider struct {
	sync.RWMutex

	client               *remote.Client
	onNewPoliciesReadyCb func()
	lastConfigs          map[string]state.ConfigCWSDD
}

var _ rules.PolicyProvider = (*RCPolicyProvider)(nil)

// NewRCPolicyProvider returns a new Remote Config based policy provider
func NewRCPolicyProvider(name string) (*RCPolicyProvider, error) {
	c, err := remote.NewClient(name, version.AgentVersion, []data.Product{data.ProductCWSDD}, time.Second*1)
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
}

func (r *RCPolicyProvider) rcConfigUpdateCallback(configs map[string]state.ConfigCWSDD) {
	r.Lock()
	r.lastConfigs = configs
	r.Unlock()

	log.Debug("new policies from remote-config policy provider")

	r.onNewPoliciesReadyCb()
}

func normalize(policy *rules.Policy) {
	// remove the version
	els := strings.SplitN(policy.Name, ".", 2)
	if len(els) > 1 {
		policy.Name = els[1]
	}
}

// LoadPolicies implements the PolicyProvider interface
func (r *RCPolicyProvider) LoadPolicies() ([]*rules.Policy, *multierror.Error) {
	var policies []*rules.Policy
	var errs *multierror.Error

	r.RLock()
	defer r.RUnlock()

	for id, c := range r.lastConfigs {
		reader := bytes.NewReader(c.Config)

		policy, err := rules.LoadPolicy(id, "remote-config", reader)
		if err != nil {
			errs = multierror.Append(errs, err)
		} else {
			normalize(policy)
			policies = append(policies, policy)
		}
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
