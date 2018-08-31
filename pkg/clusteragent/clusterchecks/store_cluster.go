// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/clusterchecks/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// clusterStore holds the state of cluster-check management.
// Lock is to be held by the users (dispatcher and api handler)
// so they can make atomic operations involving several calls.
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

func (s *clusterStore) addConfig(config integration.Config, targetNodeName string) {
	var targetNode, currentNode *nodeStore
	var foundCurrent bool
	digest := config.Digest()

	if targetNodeName != "" {
		targetNode, _ = s.getNodeStore(targetNodeName, true)
		currentNode, foundCurrent = s.getNodeStoreForDigest(digest)
	}

	// Register config
	s.digestToConfig[digest] = config

	// Dispatch to target node
	if targetNode == nil {
		return
	}
	targetNode.Lock()
	targetNode.addConfig(config)
	targetNode.Unlock()
	s.digestToNode[digest] = targetNodeName

	// Remove potential duplicate
	if foundCurrent {
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}
}

func (s *clusterStore) removeConfig(digest string) {
	currentNode, foundCurrent := s.getNodeStoreForDigest(digest)
	if foundCurrent {
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}

	delete(s.digestToConfig, digest)
}

func (s *clusterStore) getAllConfigs() []integration.Config {
	var configSlice []integration.Config
	for _, c := range s.digestToConfig {
		configSlice = append(configSlice, c)
	}
	return configSlice
}

func (s *clusterStore) getNodeConfigs(nodeName string) []integration.Config {
	node, _ := s.getNodeStore(nodeName, false)
	if node == nil {
		return nil
	}
	node.RLock()
	defer node.RUnlock()
	return node.getConfigs()
}

func (s *clusterStore) getNodeLastChange(nodeName string) int64 {
	node, _ := s.getNodeStore(nodeName, false)
	if node == nil {
		return -1
	}
	node.RLock()
	defer node.RUnlock()
	return node.lastConfigChange
}

func (s *clusterStore) storeNodeStatus(nodeName string, status types.NodeStatus) {
	node, _ := s.getNodeStore(nodeName, true)
	node.Lock()
	defer node.Unlock()
	node.lastStatus = status
	node.lastPing = timestampNow()
}

// getNodeStore retrieves the store struct for a given node name. If the node
// is not yet registered in the store, an entry will be inserted and returned,
// or an empty pointer will be returned
func (s *clusterStore) getNodeStore(nodeName string, create bool) (*nodeStore, bool) {
	node, ok := s.nodes[nodeName]
	if ok {
		return node, true
	}

	if !create {
		log.Debugf("unknown node %s, skipping", nodeName)
		return nil, false
	}

	log.Debugf("unknown node %s, registering", nodeName)
	node = newNodeStore()
	s.nodes[nodeName] = node
	return node, false
}

func (s *clusterStore) getNodeNameForDigest(digest string) (string, bool) {
	if digest == "" {
		return "", false
	}
	name, found := s.digestToNode[digest]
	return name, found
}

func (s *clusterStore) getNodeStoreForDigest(digest string) (*nodeStore, bool) {
	nodeName, found := s.getNodeNameForDigest(digest)
	if !found {
		return nil, false
	}
	return s.getNodeStore(nodeName, false)
}

func timestampNow() int64 {
	return time.Now().Unix()
}
