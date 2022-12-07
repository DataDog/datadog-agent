// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks
// +build clusterchecks

package clusterchecks

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"go.uber.org/atomic"
)

// clusterStore holds the state of cluster-check management.
// Lock is to be held by the dispatcher so it can make atomic
// operations involving several calls.
type clusterStore struct {
	sync.RWMutex
	active           bool
	digestToConfig   map[string]integration.Config            // All configurations to dispatch
	digestToNode     map[string]string                        // Node running a config
	nodes            map[string]*nodeStore                    // All nodes known to the cluster-agent
	danglingConfigs  map[string]integration.Config            // Configs we could not dispatch to any node
	endpointsConfigs map[string]map[string]integration.Config // Endpoints configs to be consumed by node agents
	idToDigest       map[check.ID]string                      // link check IDs to check configs
}

func newClusterStore() *clusterStore {
	s := &clusterStore{}
	s.reset()
	return s
}

// reset empties the store and resets all states
func (s *clusterStore) reset() {
	s.active = false
	s.digestToConfig = make(map[string]integration.Config)
	s.digestToNode = make(map[string]string)
	s.nodes = make(map[string]*nodeStore)
	s.danglingConfigs = make(map[string]integration.Config)
	s.endpointsConfigs = make(map[string]map[string]integration.Config)
	s.idToDigest = make(map[check.ID]string)
}

// getNodeStore retrieves the store struct for a given node name, if it exists
func (s *clusterStore) getNodeStore(nodeName string) (*nodeStore, bool) {
	node, ok := s.nodes[nodeName]
	return node, ok
}

// getOrCreateNodeStore retrieves the store struct for a given node name.
// If the node is not yet in the store, an entry will be inserted and returned.
func (s *clusterStore) getOrCreateNodeStore(nodeName, clientIP string) *nodeStore {
	node, ok := s.nodes[nodeName]
	if ok {
		if node.clientIP != clientIP && clientIP != "" {
			log.Debugf("Client IP changed for node %s: updating %s to %s", nodeName, node.clientIP, clientIP)
			node.clientIP = clientIP
		}
		return node
	}
	node = newNodeStore(nodeName, clientIP)
	nodeAgents.Inc(le.JoinLeaderValue)
	s.nodes[nodeName] = node
	return node
}

// clearDangling resets the danglingConfigs map to a new empty one
func (s *clusterStore) clearDangling() {
	s.danglingConfigs = make(map[string]integration.Config)
}

// nodeStore holds the state store for one node.
// Lock is to be held by the user (dispatcher)
type nodeStore struct {
	sync.RWMutex
	name           string
	heartbeat      int64
	lastStatus     types.NodeStatus
	configVersion  *atomic.Int64
	digestToConfig map[string]integration.Config
	clientIP       string
	clcRunnerStats types.CLCRunnersStats
	busyness       int
}

func newNodeStore(name, clientIP string) *nodeStore {
	return &nodeStore{
		name:           name,
		configVersion:  atomic.NewInt64(0),
		clientIP:       clientIP,
		digestToConfig: make(map[string]integration.Config),
		clcRunnerStats: types.CLCRunnersStats{},
		busyness:       defaultBusynessValue,
	}
}

func (s *nodeStore) addConfig(config integration.Config) {
	s.digestToConfig[config.Digest()] = config
	s.configVersion.Inc()
	dispatchedConfigs.Inc(s.name, le.JoinLeaderValue)
}

func (s *nodeStore) removeConfig(digest string) {
	_, found := s.digestToConfig[digest]
	if !found {
		log.Debugf("unknown digest %s, skipping", digest)
		return
	}
	delete(s.digestToConfig, digest)
	s.configVersion.Inc()
	dispatchedConfigs.Dec(s.name, le.JoinLeaderValue)
}

// AddRunnerStats stores runner stats for a check
// The nodeStore handles thread safety for this public method
func (s *nodeStore) AddRunnerStats(checkID string, stats types.CLCRunnerStats) {
	s.Lock()
	defer s.Unlock()
	s.clcRunnerStats[checkID] = stats
}

// RemoveRunnerStats deletes runner stats for a check
// The nodeStore handles thread safety for this public method
func (s *nodeStore) RemoveRunnerStats(checkID string) {
	s.Lock()
	defer s.Unlock()
	if _, found := s.clcRunnerStats[checkID]; !found {
		log.Debugf("unknown check ID %s, skipping", checkID)
		return
	}
	delete(s.clcRunnerStats, checkID)
}

// GetRunnerStats returns the runner stats of a given check
// The nodeStore handles thread safety for this public method
func (s *nodeStore) GetRunnerStats(checkID string) (types.CLCRunnerStats, error) {
	s.RLock()
	defer s.RUnlock()
	stats, found := s.clcRunnerStats[checkID]
	if !found {
		log.Debugf("unknown check ID %s", checkID)
		return stats, fmt.Errorf("check ID not found: %s", checkID)
	}
	return stats, nil
}

// GetBusyness calculates busyness of the node
// The nodeStore handles thread safety for this public method
func (s *nodeStore) GetBusyness(busynessFunc func(stats types.CLCRunnerStats) int) int {
	s.RLock()
	defer s.RUnlock()
	busyness := 0
	for _, stats := range s.clcRunnerStats {
		busyness += busynessFunc(stats)
	}
	return busyness
}

// GetMostWeightedClusterCheck returns the Cluster Check with the most weight on the node
// The nodeStore handles thread safety for this public method
func (s *nodeStore) GetMostWeightedClusterCheck(busynessFunc func(stats types.CLCRunnerStats) int) (string, int, error) {
	s.RLock()
	defer s.RUnlock()
	if len(s.clcRunnerStats) == 0 {
		log.Debugf("Node %s has no check stats", s.name)
		return "", -1, fmt.Errorf("node %s has no check stats", s.name)
	}
	firstItr := true
	checkID := ""
	checkWeight := 0
	for id, stats := range s.clcRunnerStats {
		busyness := busynessFunc(stats)
		if (busyness > checkWeight || firstItr) && stats.IsClusterCheck {
			// Only consider Cluster Checks
			checkWeight = busyness
			checkID = id
			firstItr = false
		}
	}
	if firstItr {
		log.Debugf("Node %s has no check stats for cluster checks: %v", s.name, s.clcRunnerStats)
		return "", -1, fmt.Errorf("no cluster checks found on node %s", s.name)
	}
	return checkID, checkWeight, nil
}
