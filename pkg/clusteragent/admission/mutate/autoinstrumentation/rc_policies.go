// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const remotePolicyCacheVersion = 1

type remotePolicyCache struct {
	Version int               `json:"version"`
	Configs map[string][]byte `json:"configs"`
}

var (
	apmPolicyIDPattern           = regexp.MustCompile(`^datadog/\d+/[^/]+/([^/]+)/`)
	apmPolicyPrefixPattern       = regexp.MustCompile(`^(\d+)\.`)
	apmPolicyKubernetesIDPattern = regexp.MustCompile(`^\d+\.kubernetes(?:\.|$)`)
)

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

func isKubernetesRemotePolicyPath(path string) bool {
	policyIDMatches := apmPolicyIDPattern.FindStringSubmatch(path)
	if len(policyIDMatches) <= 1 {
		return false
	}
	return apmPolicyKubernetesIDPattern.MatchString(policyIDMatches[1])
}

// subscribeRemoteConfig wires the remote-config client to the mutator so that
// SSI policies delivered over remote config are evaluated after static targets.
// RC policies are last-TRUE-wins on the wire order (default first, exceptions
// after). It is a no-op when remote config is not available, in which case the
// mutator keeps matching against its configuration baseline only. The wire
// format is the dd-wls policies document; targets do not appear on this path.
func (m *TargetMutator) subscribeRemoteConfig(client *rcclient.Client) {
	if client == nil {
		return
	}

	metrics.APMPoliciesInitialized.Set(0)
	metrics.APMPoliciesUsingCache.Set(0)
	if err := m.restoreRemotePolicyCache(); err != nil {
		log.Warnf("auto-instrumentation: failed to restore cached APM_POLICIES: %v", err)
	}

	log.Infof("auto-instrumentation: subscribing to remote config product %q for SSI policies", state.ProductApmPolicies)
	client.Subscribe(state.ProductApmPolicies, m.onRemoteConfigUpdate)

	// Subscribe before reading the current state so an update racing with this
	// read is delivered through the callback. An empty current state is
	// ambiguous: the upstream repository may not have initialized yet, so retain
	// the cached last-known-good policies until an actual update arrives.
	if len(client.GetConfigs(state.ProductApmPolicies)) > 0 {
		m.applyInitialRemoteConfigSnapshot(client)
	}
}

func (m *TargetMutator) onRemoteConfigUpdate(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	m.remoteUpdateMu.Lock()
	defer m.remoteUpdateMu.Unlock()
	m.onRemoteConfigUpdateLocked(updates, applyStateCallback)
}

func (m *TargetMutator) applyInitialRemoteConfigSnapshot(client *rcclient.Client) {
	updates := client.GetConfigs(state.ProductApmPolicies)
	m.remoteUpdateMu.Lock()
	defer m.remoteUpdateMu.Unlock()
	if m.remoteConfigInitialized.Load() {
		return
	}
	m.onRemoteConfigUpdateLocked(updates, client.UpdateApplyStatus)
}

func (m *TargetMutator) onRemoteConfigUpdateLocked(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	m.remoteConfigInitialized.Store(true)
	metrics.APMPoliciesInitialized.Set(1)
	log.Debugf("auto-instrumentation: remote config update for SSI policies: %d config(s)", len(updates))

	if len(updates) == 0 {
		// The remote snapshot is authoritative even when the local cache cannot
		// be updated. Cache maintenance must not keep stale policies active in
		// the running process.
		m.ClearRemotePolicies()
		metrics.APMPoliciesUsingCache.Set(0)
		if err := m.removeRemotePolicyCache(); err != nil {
			log.Errorf("auto-instrumentation: failed to remove cached APM_POLICIES: %v", err)
		}
		return
	}

	paths := make([]string, 0, len(updates))
	for path := range updates {
		paths = append(paths, path)
	}
	sortRemotePolicyPaths(paths)

	var kept []string
	for _, path := range paths {
		if isKubernetesRemotePolicyPath(path) {
			kept = append(kept, path)
			continue
		}
		log.Debugf("auto-instrumentation: ignoring remote config %q (not a kubernetes APM_POLICIES id)", path)
	}

	reportStatuses := func(err error) {
		for _, path := range paths {
			if err != nil && isKubernetesRemotePolicyPath(path) {
				applyStateCallback(path, state.ApplyStatus{
					State: state.ApplyStateError,
					Error: err.Error(),
				})
				continue
			}
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}

	var allPolicies []policies.Policy
	for _, path := range kept {
		parsed, err := policies.ParsePolicies(updates[path].Config)
		if err != nil {
			reportStatuses(err)
			log.Errorf("failed to parse SSI policies from remote config %q: %v", path, err)
			return
		}
		allPolicies = append(allPolicies, parsed...)
	}

	var nextPolicies *policySet
	if len(allPolicies) > 0 {
		var err error
		nextPolicies, err = m.buildRemotePolicySet(allPolicies)
		if err != nil {
			reportStatuses(err)
			log.Errorf("failed to apply SSI remote policies: %v", err)
			return
		}
	}

	configsToCache := make(map[string]state.RawConfig, len(kept))
	if len(allPolicies) > 0 {
		for _, path := range kept {
			configsToCache[path] = updates[path]
		}
	}
	if err := m.persistRemotePolicyCache(configsToCache); err != nil {
		log.Errorf("failed to persist SSI remote policies: %v", err)
	}

	m.remotePolicies.Store(nextPolicies)
	metrics.APMPoliciesUsingCache.Set(0)
	log.Infof("auto-instrumentation: applied %d SSI policies from %d remote config(s)", len(allPolicies), len(kept))
	reportStatuses(nil)
}

func remotePolicyCachePath(runPath string) string {
	return filepath.Join(runPath, "apm-policies", "kubernetes.json")
}

func (m *TargetMutator) restoreRemotePolicyCache() error {
	if m.remotePolicyCachePath == "" {
		return nil
	}
	data, err := os.ReadFile(m.remotePolicyCachePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read cache: %w", err)
	}

	var cached remotePolicyCache
	if err := json.Unmarshal(data, &cached); err != nil {
		return fmt.Errorf("decode cache: %w", err)
	}
	if cached.Version != remotePolicyCacheVersion {
		return fmt.Errorf("unsupported cache version %d", cached.Version)
	}

	paths := make([]string, 0, len(cached.Configs))
	for path := range cached.Configs {
		paths = append(paths, path)
	}
	sortRemotePolicyPaths(paths)

	var allPolicies []policies.Policy
	for _, path := range paths {
		if !isKubernetesRemotePolicyPath(path) {
			return fmt.Errorf("cache contains non-Kubernetes policy %q", path)
		}
		parsed, err := policies.ParsePolicies(cached.Configs[path])
		if err != nil {
			return fmt.Errorf("parse cached policy %q: %w", path, err)
		}
		allPolicies = append(allPolicies, parsed...)
	}
	if len(allPolicies) == 0 {
		return nil
	}
	if err := m.SetRemotePolicies(allPolicies); err != nil {
		return fmt.Errorf("apply cache: %w", err)
	}

	metrics.APMPoliciesUsingCache.Set(1)
	log.Infof("auto-instrumentation: restored %d SSI policies from cache", len(allPolicies))
	return nil
}

func (m *TargetMutator) persistRemotePolicyCache(configs map[string]state.RawConfig) error {
	if m.remotePolicyCachePath == "" {
		return nil
	}
	if len(configs) == 0 {
		return m.removeRemotePolicyCache()
	}

	rawConfigs := make(map[string][]byte, len(configs))
	for path, config := range configs {
		rawConfigs[path] = config.Config
	}
	data, err := json.Marshal(remotePolicyCache{Version: remotePolicyCacheVersion, Configs: rawConfigs})
	if err != nil {
		return fmt.Errorf("encode cache: %w", err)
	}

	dir := filepath.Dir(m.remotePolicyCachePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".kubernetes.json.tmp-")
	if err != nil {
		return fmt.Errorf("create cache temp file: %w", err)
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set cache permissions: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write cache: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache: %w", err)
	}
	if err := os.Rename(tmpPath, m.remotePolicyCachePath); err != nil {
		return fmt.Errorf("replace cache: %w", err)
	}
	removeTemp = false
	return nil
}

func (m *TargetMutator) removeRemotePolicyCache() error {
	if m.remotePolicyCachePath == "" {
		return nil
	}
	err := os.Remove(m.remotePolicyCachePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove cache: %w", err)
	}
	return nil
}
