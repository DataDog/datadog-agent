// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type rcTargetsPayload struct {
	Targets []Target `json:"targets"`
}

type rcTargetProvider struct {
	client        *rcclient.Client
	config        *Config
	wmeta         workloadmeta.Component
	imageResolver imageresolver.Resolver

	mu      sync.RWMutex
	current *TargetMutator
}

func newRCTargetProvider(client *rcclient.Client, config *Config, wmeta workloadmeta.Component, imageResolver imageresolver.Resolver) (*rcTargetProvider, error) {
	if client == nil {
		return nil, nil
	}

	provider := &rcTargetProvider{
		client:        client,
		config:        config,
		wmeta:         wmeta,
		imageResolver: imageResolver,
	}
	client.Subscribe(state.ProductSSITargets, provider.onUpdate)
	provider.onUpdate(client.GetConfigs(state.ProductSSITargets), client.UpdateApplyStatus)
	return provider, nil
}

func (p *rcTargetProvider) Current() *TargetMutator {
	if p == nil {
		return nil
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.current
}

func (p *rcTargetProvider) onUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	filteredUpdates := make(map[string]state.RawConfig, len(updates))
	for path, update := range updates {
		if strings.Contains(path, "/DEBUG/ssi-targets-test/") {
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

	log.Infof("SSI targets updated: %v", updates)

	targets := make([]Target, 0, len(updates))
	for path, update := range updates {
		payload, err := parseRCTargets(update.Config)
		if err != nil {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			log.Errorf("failed to parse SSI targets from remote config %q: %v", path, err)
			return
		}
		targets = append(targets, payload.Targets...)
	}

	rcConfig := *p.config
	rcInstrumentation := *p.config.Instrumentation
	rcInstrumentation.Enabled = true
	rcInstrumentation.EnabledNamespaces = nil
	rcInstrumentation.LibVersions = nil
	rcInstrumentation.Targets = targets
	rcConfig.Instrumentation = &rcInstrumentation

	mutator, err := newTargetMutatorWithTargets(&rcConfig, p.wmeta, p.imageResolver, targets, true)
	if err != nil {
		for path := range updates {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
		log.Errorf("failed to build SSI remote target mutator: %v", err)
		return
	}

	p.mu.Lock()
	p.current = mutator
	p.mu.Unlock()

	for path := range updates {
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}

func parseRCTargets(raw []byte) (*rcTargetsPayload, error) {
	var payload rcTargetsPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unable to parse SSI remote targets payload: %w", err)
	}
	return &payload, nil
}
