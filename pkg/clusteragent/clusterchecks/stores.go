// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// clusterStore holds the state of cluster-check management.
// Lock is to be held by the dispatcher so it can make atomic
// operations involving several calls.
type clusterStore struct {
	sync.RWMutex
	digestToConfig map[string]integration.Config // All configurations to dispatch
	digestToNode   map[string]string             // Node running a config
	nodes          map[string]*nodeStore         // All nodes known to the cluster-agent
}

func newClusterStore() *clusterStore {
	return &clusterStore{
		digestToConfig: make(map[string]integration.Config),
		digestToNode:   make(map[string]string),
		nodes:          make(map[string]*nodeStore),
	}
}

// getNodeStore retrieves the store struct for a given node name, if it exists
func (s *clusterStore) getNodeStore(nodeName string) (*nodeStore, bool) {
	node, ok := s.nodes[nodeName]
	return node, ok
}

// getOrCreateNodeStore retrieves the store struct for a given node name.
// If the node is not yet in the store, an entry will be inserted and returned.
func (s *clusterStore) getOrCreateNodeStore(nodeName string) *nodeStore {
	node, ok := s.nodes[nodeName]
	if ok {
		return node
	}

	log.Debugf("unknown node %s, registering", nodeName)
	node = newNodeStore()
	s.nodes[nodeName] = node
	return node
}

// nodeStore holds the state store for one node.
// Lock is to be held by the user (dispatcher)
type nodeStore struct {
	sync.RWMutex
	lastPing         int64
	lastStatus       types.NodeStatus
	lastConfigChange int64
	digestToConfig   map[string]integration.Config
}

func newNodeStore() *nodeStore {
	return &nodeStore{
		digestToConfig: make(map[string]integration.Config),
	}
}

func (s *nodeStore) addConfig(config integration.Config) {
	s.lastConfigChange = timestampNow()
	s.digestToConfig[config.Digest()] = config
}

func (s *nodeStore) removeConfig(digest string) {
	_, found := s.digestToConfig[digest]
	if !found {
		log.Debug("unknown digest %s, skipping", digest)
		return
	}
	s.lastConfigChange = timestampNow()
	delete(s.digestToConfig, digest)
}
