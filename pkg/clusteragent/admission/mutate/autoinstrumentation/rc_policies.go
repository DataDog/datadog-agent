// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"strings"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// rcPolicyProvider subscribes to remote-config SSI policies and compiles them
// into a policy-driven TargetMutator. Targets no longer appear on this path;
// the wire format is the dd-wls policies document.
type rcPolicyProvider struct {
	client        *rcclient.Client
	config        *Config
	wmeta         workloadmeta.Component
	imageResolver imageresolver.Resolver

	mu      sync.RWMutex
	current *TargetMutator
}

func newRCPolicyProvider(client *rcclient.Client, config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver) (*rcPolicyProvider, error) {
	if client == nil {
		return nil, nil
	}

	provider := &rcPolicyProvider{
		client:        client,
		config:        config,
		wmeta:         wmeta,
		imageResolver: imageResolver,
	}
	client.Subscribe(state.ProductSSITargets, provider.onUpdate)
	provider.onUpdate(client.GetConfigs(state.ProductSSITargets), client.UpdateApplyStatus)
	return provider, nil
}

func (p *rcPolicyProvider) Current() *TargetMutator {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}

func (p *rcPolicyProvider) onUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	filteredUpdates := make(map[string]state.RawConfig, len(updates))
	for path, update := range updates {
		if strings.Contains(path, "/DEBUG/") {
			filteredUpdates[path] = update
		}
	}
	updates = filteredUpdates

	if len(updates) == 0 {
		p.mu.Lock()
		p.current = nil
		p.mu.Unlock()
		return
	}

	log.Infof("SSI policies updated: %v", updates)

	var allPolicies []policies.Policy
	for path, update := range updates {
		parsed, err := policies.ParsePolicies(update.Config)
		if err != nil {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			log.Errorf("failed to parse SSI policies from remote config %q: %v", path, err)
			return
		}
		allPolicies = append(allPolicies, parsed...)
	}

	rcConfig := *p.config
	rcInstrumentation := *p.config.Instrumentation
	rcInstrumentation.Enabled = true
	rcInstrumentation.EnabledNamespaces = nil
	rcInstrumentation.LibVersions = nil
	rcInstrumentation.Targets = nil
	rcConfig.Instrumentation = &rcInstrumentation

	mutator, err := newTargetMutatorFromPolicies(&rcConfig, p.wmeta, p.imageResolver, nil, allPolicies, true)
	if err != nil {
		for path := range updates {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
		log.Errorf("failed to build SSI remote policy mutator: %v", err)
		return
	}

	p.mu.Lock()
	p.current = mutator
	p.mu.Unlock()

	for path := range updates {
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}
