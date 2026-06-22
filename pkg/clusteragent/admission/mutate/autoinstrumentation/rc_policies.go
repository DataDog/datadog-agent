// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"sort"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

// subscribeRemoteConfig wires the remote-config client to the mutator so that
// SSI policies delivered over remote config are layered on top of the
// configuration baseline. It is a no-op when remote config is not available,
// in which case the mutator keeps matching against its configuration baseline
// only. The wire format is the dd-wls policies document; targets do not appear
// on this path.
func (m *TargetMutator) subscribeRemoteConfig(client *rcclient.Client) {
	if client == nil {
		return
	}

	log.Infof("auto-instrumentation: subscribing to remote config product %q for SSI policies", state.ProductApmPolicies)
	client.Subscribe(state.ProductApmPolicies, m.onRemoteConfigUpdate)
	// Apply whatever the client already has, so we don't wait for the next
	// update to reflect the current state.
	m.onRemoteConfigUpdate(client.GetConfigs(state.ProductApmPolicies), client.UpdateApplyStatus)
}

func (m *TargetMutator) onRemoteConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Debugf("auto-instrumentation: remote config update for SSI policies: %d config(s)", len(updates))

	if len(updates) == 0 {
		m.ClearRemotePolicies()
		return
	}

	// Sort paths so that policy precedence is deterministic across updates and
	// Cluster Agent instances: the matcher is first-match-wins, so an
	// inconsistent iteration order over the map would flip allow/deny decisions
	// whenever two policy files have overlapping rules.
	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	var allPolicies []policies.Policy
	for _, path := range paths {
		parsed, err := policies.ParsePolicies(updates[path].Config)
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

	if err := m.SetRemotePolicies(allPolicies); err != nil {
		for path := range updates {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
		log.Errorf("failed to apply SSI remote policies: %v", err)
		return
	}

	log.Infof("auto-instrumentation: applied %d SSI policies from %d remote config(s)", len(allPolicies), len(updates))
	for path := range updates {
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}
