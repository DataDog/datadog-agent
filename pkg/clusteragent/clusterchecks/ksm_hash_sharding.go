// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks

package clusterchecks

import (
	"errors"
	"fmt"
	"maps"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	ksmsharding "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/sharding"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// hashSharding is the validated, canonical hash-sharding config of a KSM
// instance.
type hashSharding struct {
	criteria []string
	count    int
}

// ksmHashShardingManager shards a KSM check by rendezvous-hashing the dimensions
// named in shard_criteria, as opposed to ksmShardingManager's fixed
// pods/nodes/others split. It is a peer shardingStrategy rather than a branch
// inside that one so each scheme has a single code path and an accurate label().
//
// The KSM strategies are mutually exclusive: this one requires valid
// shard_criteria, while resource-group sharding declines any hash request.
type ksmHashShardingManager struct {
	enabled bool
}

// newKSMHashShardingManager creates a new KSM hash sharding manager.
func newKSMHashShardingManager(enabled bool) *ksmHashShardingManager {
	return &ksmHashShardingManager{
		enabled: enabled,
	}
}

// isEnabled returns whether KSM sharding is enabled.
func (m *ksmHashShardingManager) isEnabled() bool {
	return m.enabled
}

// label satisfies shardingStrategy.
func (m *ksmHashShardingManager) label() string {
	return "hash-sharded KSM"
}

// shouldShard satisfies shardingStrategy; see shouldShardKSMHashCheck.
func (m *ksmHashShardingManager) shouldShard(config integration.Config) bool {
	return m.shouldShardKSMHashCheck(config)
}

// createShardedConfigs satisfies shardingStrategy; see createShardedKSMHashConfigs.
func (m *ksmHashShardingManager) createShardedConfigs(config integration.Config) ([]integration.Config, error) {
	return m.createShardedKSMHashConfigs(config)
}

// parseHashSharding reports whether and how an instance opts into hash
// sharding. It returns (nil, nil) when shard_criteria is absent.
func parseHashSharding(data integration.Data) (*hashSharding, error) {
	var s struct {
		ShardCriteria []string `yaml:"shard_criteria"`
		ShardCount    int      `yaml:"shard_count"`
	}

	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("failed to parse KSM instance config: %w", err)
	}

	if len(s.ShardCriteria) == 0 {
		return nil, nil
	}

	if s.ShardCount < 2 {
		return nil, fmt.Errorf("hash sharding requires shard_count >= 2, got %d", s.ShardCount)
	}

	criteria := make([]string, 0, len(s.ShardCriteria))
	seen := make(map[string]struct{}, len(s.ShardCriteria))
	for _, criterion := range s.ShardCriteria {
		if criterion != ksmsharding.CriterionNamespace && criterion != ksmsharding.CriterionResource {
			return nil, fmt.Errorf("unknown shard criterion %q, valid values are [%s, %s]", criterion, ksmsharding.CriterionNamespace, ksmsharding.CriterionResource)
		}
		if _, duplicate := seen[criterion]; duplicate {
			return nil, fmt.Errorf("shard criterion %q specified more than once", criterion)
		}
		seen[criterion] = struct{}{}
		criteria = append(criteria, criterion)
	}

	// Canonical order: so that all produced configs look the same.
	sort.Strings(criteria)

	return &hashSharding{criteria: criteria, count: s.ShardCount}, nil
}

// shouldShardKSMHashCheck determines whether a KSM check has a valid hash
// sharding request. Invalid requests are logged and scheduled unsharded.
func (m *ksmHashShardingManager) shouldShardKSMHashCheck(config integration.Config) bool {
	if !m.enabled || !isKSMCheck(config) {
		return false
	}
	// Sharding only makes sense for cluster checks (dispatched to CLC runners).
	// If ClusterCheck is false, the check runs locally on the DCA.
	if !config.ClusterCheck {
		log.Warnf("KSM hash sharding requires cluster_check: true, but got cluster_check: false")
		return false
	}

	shardable, _, err := classifyKSMInstances(config)
	if err != nil {
		log.Warnf("KSM hash sharding disabled: %v", err)
		return false
	}

	hashShards, err := parseHashSharding(shardable)
	if err != nil {
		log.Warnf("KSM hash sharding disabled: %v", err)
		return false
	}

	// No request leaves the instance available to resource-group sharding.
	if hashShards == nil {
		return false
	}

	log.Infof("KSM hash sharding enabled: will create %d shards (criteria=%v)", hashShards.count, hashShards.criteria)
	return true
}

// createShardedKSMHashConfigs creates the hash-sharded KSM configurations for a
// check.
func (m *ksmHashShardingManager) createShardedKSMHashConfigs(
	baseConfig integration.Config,
) ([]integration.Config, error) {
	shardable, passthrough, err := classifyKSMInstances(baseConfig)
	if err != nil {
		return nil, err
	}

	hashShards, err := parseHashSharding(shardable)
	if err != nil {
		return nil, err
	}
	if hashShards == nil {
		return nil, errors.New("KSM instance no longer requests hash sharding")
	}

	return m.createKSMHashShards(baseConfig, shardable, passthrough, hashShards), nil
}

// createKSMHashShards creates one config per hash shard, each a copy of the
// shardable instance with its own shard_id and the shard_criteria.
// Every shard must see identical criteria and shard_count, or
// they would disagree about ownership.
//
// Any cluster_aggregates_only instance is dispatched as its own config rather
// than riding along with a shard.
func (m *ksmHashShardingManager) createKSMHashShards(
	baseConfig integration.Config,
	shardableInstance integration.Data,
	passthrough []integration.Data,
	hashShards *hashSharding,
) []integration.Config {
	// Parse the shardable instance config (not necessarily baseConfig.Instances[0])
	var instance map[string]interface{}
	if err := yaml.Unmarshal(shardableInstance, &instance); err != nil {
		log.Warnf("Failed to unmarshal shardable KSM instance config: %v", err)
		instance = make(map[string]interface{})
	}

	if len(passthrough) > 0 {
		// Extras are dropped rather than dispatched.
		if len(passthrough) > 1 {
			log.Errorf("KSM sharding: %d %s instances configured; each does a full-pod watch and emits the .total family, which double-counts. Keeping the first and dropping the other %d; configure exactly one.", len(passthrough), clusterAggregatesOnlyMode, len(passthrough)-1)
		}

		// Worse here than under resource-group sharding, where only the pods
		// shard double-counts: every hash shard carries the full collector set,
		// so all shard_count of them emit .total.
		//
		// Dispatched anyway, because refusing wouldn't fix it — an error falls
		// through to unsharded scheduling, which still ships both instances
		// together and still double-counts .total (2x instead of N+1x), minus
		// the sharding. Only the operator can fix this, so the log is it.
		if message, isError, ok := suppressionDiagnostic(shardableInstance); !ok {
			message = fmt.Sprintf("%s All %d hash shards collect the same resources, so each one contributes to the inflated .total.", message, hashShards.count)
			if isError {
				log.Error(message)
			} else {
				log.Warn(message)
			}
		}
	}

	var aggregate integration.Data
	if len(passthrough) > 0 {
		var err error
		aggregate, err = withSkipLeaderElection(passthrough[0])
		if err != nil {
			log.Warnf("Failed to prepare %s instance: %v", clusterAggregatesOnlyMode, err)
		}
	}

	configs := make([]integration.Config, 0, hashShards.count+1)
	for i := 0; i < hashShards.count; i++ {
		shard := maps.Clone(instance)
		shard["shard_criteria"] = hashShards.criteria
		shard["shard_count"] = hashShards.count
		shard["shard_id"] = i
		// Enable skip_leader_election for cluster checks running on CLC runners
		shard["skip_leader_election"] = true

		data, err := yaml.Marshal(shard)
		if err != nil {
			log.Warnf("Failed to marshal KSM hash shard %d: %v", i, err)
			continue
		}

		config := shardConfigShell(baseConfig)
		config.Instances = []integration.Data{data}
		configs = append(configs, config)
	}

	if aggregate != nil {
		aggregateConfig := shardConfigShell(baseConfig)
		aggregateConfig.Instances = []integration.Data{aggregate}
		configs = append(configs, aggregateConfig)
	}

	log.Infof("Created %d hash-sharded KSM configs (criteria=%v), standalone %s config: %v",
		hashShards.count, hashShards.criteria, clusterAggregatesOnlyMode, aggregate != nil)

	return configs
}
