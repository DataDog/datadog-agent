// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"regexp"
	"sort"
	"strconv"
	"time"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

var (
	apmPolicyIDPattern     = regexp.MustCompile(`^datadog/\d+/[^/]+/([^/]+)/`)
	apmPolicyPrefixPattern = regexp.MustCompile(`^(\d+)\.`)
)

// rcInjectAllWaitTimeout bounds how long SSI inject-all is withheld while
// waiting for the first remote-config answer. A var so tests can shorten it.
var rcInjectAllWaitTimeout = time.Minute

// sortRemotePolicyPaths preserves the numeric-prefix ordering used by the
// APM_POLICIES product. It is intentionally local: Remote Config paths are
// otherwise opaque and this must not be treated as a generic RC convention.
func sortRemotePolicyPaths(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		leftOrder := remotePolicyPathOrder(paths[i])
		rightOrder := remotePolicyPathOrder(paths[j])
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return paths[i] < paths[j]
	})
}

func remotePolicyPathOrder(path string) int {
	policyIDMatches := apmPolicyIDPattern.FindStringSubmatch(path)
	if len(policyIDMatches) <= 1 {
		return 0
	}

	prefixMatches := apmPolicyPrefixPattern.FindStringSubmatch(policyIDMatches[1])
	if len(prefixMatches) <= 1 {
		return 0
	}

	order, err := strconv.Atoi(prefixMatches[1])
	if err != nil {
		return 0
	}
	return order
}

// subscribeRemoteConfig wires the remote-config client to the mutator so that
// SSI policies delivered over remote config are evaluated after static targets.
// RC policies are last-TRUE-wins on the wire order (default first, exceptions
// after). It is a no-op when remote config is not available, in which case the
// mutator keeps matching against its configuration baseline only. The wire
// format is the dd-wls policies document; targets do not appear on this path.
func (m *TargetMutator) subscribeRemoteConfig(client *rcclient.Client) {
	if client == nil {
		m.allowInjectAll.Store(true)
		return
	}

	log.Infof("auto-instrumentation: subscribing to remote config product %q for SSI policies", state.ProductApmPolicies)

	// WithInitialUpdate: a plain subscription only fires on changes, so "the
	// backend has no policies for us" would never reach onRemoteConfigUpdate --
	// and that callback is what releases inject-all.
	client.SubscribeAll(
		state.ProductApmPolicies,
		rcclient.NewUpdateListener(m.onRemoteConfigUpdate),
		rcclient.WithInitialUpdate(),
	)

	// Bound the wait: if remote config never answers, instrument rather than
	// silently withhold SSI from a configuration that asked for it.
	time.AfterFunc(rcInjectAllWaitTimeout, func() {
		if m.allowInjectAll.CompareAndSwap(false, true) {
			log.Warnf("auto-instrumentation: no remote config answer for %q after %s, applying SSI inject-all",
				state.ProductApmPolicies, rcInjectAllWaitTimeout)
		}
	})
}

func (m *TargetMutator) enableInjectAll() {
	if m.allowInjectAll.CompareAndSwap(false, true) {
		log.Infof("auto-instrumentation: first remote config snapshot for SSI policies received")
	}
}

func (m *TargetMutator) onRemoteConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	log.Debugf("auto-instrumentation: remote config update for SSI policies: %d config(s)", len(updates))

	if len(updates) == 0 {
		m.ClearRemotePolicies()
		m.enableInjectAll()
		return
	}

	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sortRemotePolicyPaths(paths)
	reportApplyError := func(err error) {
		for _, path := range paths {
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
		}
	}

	var allPolicies []policies.Policy
	for _, path := range paths {
		parsed, err := policies.ParsePolicies(updates[path].Config)
		if err != nil {
			reportApplyError(err)
			log.Errorf("failed to parse SSI policies from remote config %q: %v", path, err)
			return
		}
		allPolicies = append(allPolicies, parsed...)
	}

	if err := m.SetRemotePolicies(allPolicies); err != nil {
		reportApplyError(err)
		log.Errorf("failed to apply SSI remote policies: %v", err)
		return
	}

	log.Infof("auto-instrumentation: applied %d SSI policies from %d remote config(s)", len(allPolicies), len(updates))
	for path := range updates {
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}
	m.enableInjectAll()
}
