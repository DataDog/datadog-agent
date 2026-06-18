// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// subscribeRemoteConfig wires the remote-config client to the mutator so that
// SSI policies delivered over remote config are layered on top of the
// configuration baseline. It is a no-op when remote config is not available,
// in which case the mutator keeps matching against its configuration baseline
// only. Targets no longer appear on this path; the wire format is the dd-wls
// policies document.
func (m *TargetMutator) subscribeRemoteConfig(client *rcclient.Client) {
	if client == nil {
		return
	}

	client.Subscribe(state.ProductSSITargets, m.onRemoteConfigUpdate)
	m.onRemoteConfigUpdate(client.GetConfigs(state.ProductSSITargets), client.UpdateApplyStatus)
}

func (m *TargetMutator) onRemoteConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	filteredUpdates := make(map[string]state.RawConfig, len(updates))
	for path, update := range updates {
		if strings.Contains(path, "/DEBUG/") {
			filteredUpdates[path] = update
		}
	}
	updates = filteredUpdates

	if len(updates) == 0 {
		m.ClearRemotePolicies()
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

	for path := range updates {
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
}
