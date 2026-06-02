// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"maps"
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

// store holds useful mappings for the AD
type store struct {
	// nameToJMXMetrics stores the MetricConfig for checks, keyed by check name.
	nameToJMXMetrics map[string]integration.Data

	// idsOfChecksWithSecrets maps the IDs of check instances received from the
	// Cluster Agent that contain resolved secrets to the IDs that they had
	// before decrypting their secrets.
	// This is needed because the ID of a check instance with secrets changes
	// when its secrets are decrypted. In some configurations the Cluster Agent
	// does not resolve the secrets (config option secret_backend_skip_checks
	// set to true), but the Cluster Check Runner does. We need this mapping to
	// be able to match the 2 IDs.
	idsOfChecksWithSecrets map[checkid.ID]checkid.ID

	// m is a Mutex protecting access to all fields in this type
	m sync.RWMutex
}

// newStore creates a store
func newStore() *store {
	s := store{
		nameToJMXMetrics:       make(map[string]integration.Data),
		idsOfChecksWithSecrets: make(map[checkid.ID]checkid.ID),
	}

	return &s
}

// setJMXMetricsForConfigName stores the jmx metrics config for a config name
func (s *store) setJMXMetricsForConfigName(config string, metrics integration.Data) {
	s.m.Lock()
	defer s.m.Unlock()
	s.nameToJMXMetrics[config] = metrics
}

// getJMXMetricsForConfigName returns the stored JMX metrics for a config name
func (s *store) getJMXMetricsForConfigName(config string) integration.Data {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.nameToJMXMetrics[config]
}

func (s *store) setIDsOfChecksWithSecrets(checkIDs map[checkid.ID]checkid.ID) {
	s.m.Lock()
	defer s.m.Unlock()

	maps.Copy(s.idsOfChecksWithSecrets, checkIDs)
}

func (s *store) getIDOfCheckWithEncryptedSecrets(idCheckWithResolvedSecrets checkid.ID) checkid.ID {
	s.m.Lock()
	defer s.m.Unlock()

	return s.idsOfChecksWithSecrets[idCheckWithResolvedSecrets]
}

func (s *store) deleteMappingsOfCheckIDsWithSecrets(checkIDs []checkid.ID) {
	s.m.Lock()
	defer s.m.Unlock()

	for _, checkID := range checkIDs {
		delete(s.idsOfChecksWithSecrets, checkID)
	}
}
