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

// nodeStore holds the state store for one node. Lock is to be held
// by the user (clusterStore)
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

func (s *nodeStore) getConfigs() []integration.Config {
	var configSlice []integration.Config
	for _, c := range s.digestToConfig {
		configSlice = append(configSlice, c)
	}
	return configSlice
}
