// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/connfilter"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	remoteConfigScheduledType = "scheduled"
	remoteConfigDynamicType   = "dynamic"
	maxRemoteFilters          = 200
)

type remoteConfigEnvelope struct {
	Type         string               `json:"type"`
	TestConfigID string               `json:"test_config_id"`
	Config       *remoteDynamicConfig `json:"config"`
}

type remoteDynamicConfig struct {
	Filters []remoteFilterConfig `json:"filters"`
}

type remoteFilterConfig struct {
	Type                connfilter.FilterType              `json:"type"`
	MatchDomain         string                             `json:"match_domain"`
	MatchDomainStrategy connfilter.MatchDomainStrategyType `json:"match_domain_strategy"`
	MatchIP             string                             `json:"match_ip"`
}

func (c remoteFilterConfig) toConnFilterConfig(testConfigID string) connfilter.Config {
	return connfilter.Config{
		Type:                c.Type,
		MatchDomain:         c.MatchDomain,
		MatchDomainStrategy: c.MatchDomainStrategy,
		MatchIP:             c.MatchIP,
		TestConfigID:        testConfigID,
	}
}

// dynamicRemoteConfigState contains valid dynamic configs indexed by RC path.
// The product contract permits only one active path; retaining the map lets an
// invalid replacement preserve the last valid value for that same path.
type dynamicRemoteConfigState map[string][]connfilter.Config

// UpdateRemoteConfig applies a full NETWORK_PATH snapshot to the dynamic filter layer.
func (s *npCollectorImpl) UpdateRemoteConfig(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	// A noop collector still validates and acknowledges policy. Dynamic RC does
	// not enable collection, and a fresh snapshot is delivered after a restart
	// if a collector is enabled later.
	if s.collectorConfigs == nil {
		s.collectorConfigs = &collectorConfigs{}
	}
	if s.remoteConfigState == nil {
		s.remoteConfigState = make(dynamicRemoteConfigState)
	}

	seenDynamicPaths := make(map[string]struct{})
	validPaths := make(map[string]struct{})
	for path, rawConfig := range updates {
		filters, dynamic, err := parseRemoteDynamicConfig(rawConfig.Config)
		if !dynamic {
			continue
		}
		seenDynamicPaths[path] = struct{}{}
		if err != nil {
			s.loggerErrorf("Skipping invalid dynamic NETWORK_PATH update %s: %v", path, err)
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			continue
		}
		s.remoteConfigState[path] = filters
		validPaths[path] = struct{}{}
	}

	for path := range s.remoteConfigState {
		if _, found := seenDynamicPaths[path]; !found {
			delete(s.remoteConfigState, path)
		}
	}

	if len(s.remoteConfigState) > 1 {
		err := fmt.Errorf("multiple dynamic NETWORK_PATH configs match this Agent: expected at most one, got %d", len(s.remoteConfigState))
		for path := range s.remoteConfigState {
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		}
		s.replaceRemoteFilters(nil)
		return
	}

	var remoteFilters []connfilter.Config
	for path, filters := range s.remoteConfigState {
		remoteFilters = filters
		if _, validInSnapshot := validPaths[path]; validInSnapshot {
			applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
	s.replaceRemoteFilters(remoteFilters)
}

// parseRemoteDynamicConfig returns dynamic=false for scheduled configs so the
// scheduled provider remains the sole owner of their apply status.
func parseRemoteDynamicConfig(raw []byte) ([]connfilter.Config, bool, error) {
	var header struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, true, fmt.Errorf("invalid Network Path config: %w", err)
	}
	if header.Type == remoteConfigScheduledType {
		return nil, false, nil
	}
	if header.Type != remoteConfigDynamicType {
		return nil, true, fmt.Errorf("unsupported Network Path config type %q", header.Type)
	}

	var envelope remoteConfigEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, true, fmt.Errorf("invalid dynamic Network Path config: %w", err)
	}
	if strings.TrimSpace(envelope.TestConfigID) == "" {
		return nil, true, errors.New("invalid dynamic Network Path config: test_config_id is required")
	}
	if envelope.Config == nil {
		return nil, true, errors.New("invalid dynamic Network Path config: config must be provided")
	}
	if len(envelope.Config.Filters) == 0 {
		return nil, true, errors.New("invalid dynamic Network Path config: config.filters must contain at least one item")
	}
	if len(envelope.Config.Filters) > maxRemoteFilters {
		return nil, true, fmt.Errorf("invalid dynamic Network Path config: config.filters must contain at most %d items", maxRemoteFilters)
	}
	filters := make([]connfilter.Config, len(envelope.Config.Filters))
	for i, filterConfig := range envelope.Config.Filters {
		if filterConfig.MatchDomain == "" && filterConfig.MatchIP == "" {
			return nil, true, fmt.Errorf("invalid dynamic Network Path config at filters[%d]: match_domain or match_ip is required", i)
		}
		filters[i] = filterConfig.toConnFilterConfig(envelope.TestConfigID)
	}
	_, validationErrors := connfilter.NewConnFilter(filters, "", false)
	if len(validationErrors) > 0 {
		return nil, true, fmt.Errorf("invalid dynamic Network Path config: %w", errors.Join(validationErrors...))
	}
	return filters, true, nil
}

func (s *npCollectorImpl) replaceRemoteFilters(remoteFilters []connfilter.Config) {
	combined := make([]connfilter.Config, 0, len(s.collectorConfigs.filterConfig)+len(remoteFilters))
	combined = append(combined, s.collectorConfigs.filterConfig...)
	combined = append(combined, remoteFilters...)
	filter, errs := connfilter.NewConnFilter(combined, s.collectorConfigs.ddSite, s.collectorConfigs.monitorIPWithoutDomain)
	if len(errs) > 0 {
		s.loggerErrorf("connection filter errors while applying dynamic NETWORK_PATH config: %s", errors.Join(errs...))
	}
	s.filterMutex.Lock()
	s.filter = filter
	s.filterMutex.Unlock()
}

func (s *npCollectorImpl) loggerErrorf(format string, args ...interface{}) {
	if s.logger != nil {
		s.logger.Errorf(format, args...)
	}
}
