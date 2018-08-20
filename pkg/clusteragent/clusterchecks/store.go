// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build clusterchecks

package clusterchecks

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// store holds the state of cluster-check management.
// It is written to by the dispatcher and read from by the api handler
type store struct {
	m             sync.RWMutex
	digest2Config map[string]integration.Config // All configurations to dispatch
}

func newStore() *store {
	return &store{
		digest2Config: make(map[string]integration.Config),
	}
}

func (s *store) addConfig(config integration.Config) {
	s.m.Lock()
	defer s.m.Unlock()

	s.digest2Config[config.Digest()] = config
}

func (s *store) removeConfig(digest string) {
	s.m.Lock()
	defer s.m.Unlock()

	_, found := s.digest2Config[digest]
	if !found {
		log.Debug("unknown digest %s, skipping", digest)
		return
	}
	delete(s.digest2Config, digest)
}

func (s *store) getAllConfigs() []integration.Config {
	s.m.RLock()
	defer s.m.RUnlock()

	var configSlice []integration.Config
	for _, c := range s.digest2Config {
		configSlice = append(configSlice, c)
	}
	return configSlice
}
