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
// It is written to by the dispatcher and read from by the api handler
type clusterStore struct {
	m              sync.RWMutex
	digestToConfig map[string]integration.Config // All configurations to dispatch
	digestToNode   map[string]string             // Node running a config
	nodes          map[string]*nodeStore         // All nodes known to the cluster-agent
}

func newClusterStore() *clusterStore {
	return &clusterStore{
		digestToConfig: make(map[string]integration.Config),
		nodes:          make(map[string]*nodeStore),
	}
}

func (s *clusterStore) addConfig(config integration.Config, targetNodeName string) {
	s.m.Lock()
	defer s.m.Unlock()

	digest := config.Digest()
	s.digestToConfig[digest] = config

	// Dispatch to target node
	if targetNodeName == "" {
		return
	}
	targetNode, _ := s.getNodeStore(targetNodeName, true)
	targetNode.Lock()
	targetNode.addConfig(config)
	targetNode.Unlock()

	currentNodeName := s.digestToNode[digest]
	s.digestToNode[digest] = targetNodeName

	// Remove potential duplicate
	if currentNodeName != "" {
		currentNode, found := s.getNodeStore(currentNodeName, false)
		if !found {
			return
		}
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}

}

func (s *clusterStore) removeConfig(digest string) {
	s.m.Lock()
	defer s.m.Unlock()

	_, found := s.digestToConfig[digest]
	if !found {
		log.Debug("unknown digest %s, skipping", digest)
		return
	}

	currentNodeName, found := s.digestToNode[digest]
	if found {
		currentNode, found := s.getNodeStore(currentNodeName, false)
		if !found {
			return
		}
		currentNode.Lock()
		currentNode.removeConfig(digest)
		currentNode.Unlock()
	}

	delete(s.digestToConfig, digest)
}

func (s *clusterStore) getAllConfigs() []integration.Config {
	s.m.RLock()
	defer s.m.RUnlock()

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
	s.m.RLock()
	node, ok := s.nodes[nodeName]
	s.m.RUnlock()
	if ok {
		return node, true
	}

	if !create {
		log.Debug("unknown node %s, skipping", nodeName)
		return nil, false
	}

	log.Debug("unknown node %s, registering", nodeName)
	s.m.Lock()
	defer s.m.Unlock()
	node = &nodeStore{}
	s.nodes[nodeName] = node
	return node, false
}

func timestampNow() int64 {
	return time.Now().Unix()
}
