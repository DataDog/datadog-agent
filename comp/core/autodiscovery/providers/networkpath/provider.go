// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkpath provides Network Path scheduled test configs from Remote Configuration.
package networkpath

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	networkpathcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkpath"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	scheduledType = "scheduled"
	dynamicType   = "dynamic"
	configSource  = names.NetworkPathRemoteConfig + ":scheduled"
)

// Provider receives scheduled Network Path tests from Remote Configuration.
type Provider struct {
	stateMutex    sync.RWMutex
	configChanges chan integration.ConfigChanges
	shutdownCh    chan struct{}
	closeOnce     sync.Once
	closed        bool

	activeByPath map[string][]integration.Config
	configErrors map[string]types.ErrorMsgSet
}

type remoteConfigEnvelope struct {
	Type         string           `json:"type"`
	TestConfigID string           `json:"test_config_id"`
	Config       *scheduledConfig `json:"config"`
}

type scheduledConfig struct {
	Tests []endpointConfig `json:"tests"`
}

type endpointConfig struct {
	Hostname           string   `json:"hostname"`
	Port               *int     `json:"port,omitempty"`
	Protocol           *string  `json:"protocol,omitempty"`
	MaxTTL             *int     `json:"max_ttl,omitempty"`
	TimeoutMS          *int64   `json:"timeout_ms,omitempty"`
	IntervalSec        *int     `json:"interval_sec,omitempty"`
	SourceService      string   `json:"source_service,omitempty"`
	DestinationService string   `json:"destination_service,omitempty"`
	TCPMethod          *string  `json:"tcp_method,omitempty"`
	TracerouteQueries  *int     `json:"traceroute_queries,omitempty"`
	E2eQueries         *int     `json:"e2e_queries,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

type networkPathInstanceConfig struct {
	// TestConfigID identifies the scheduled Network Path test config that produced this instance.
	TestConfigID string `yaml:"test_config_id"`

	Hostname string  `yaml:"hostname"`
	Port     *uint16 `yaml:"port,omitempty"`
	Protocol string  `yaml:"protocol,omitempty"`

	MaxTTL                *uint8 `yaml:"max_ttl,omitempty"`
	Timeout               *int64 `yaml:"timeout,omitempty"`
	MinCollectionInterval *int   `yaml:"min_collection_interval,omitempty"`

	SourceService      string `yaml:"source_service,omitempty"`
	DestinationService string `yaml:"destination_service,omitempty"`
	TCPMethod          string `yaml:"tcp_method,omitempty"`

	TracerouteQueries *int     `yaml:"traceroute_queries,omitempty"`
	E2eQueries        *int     `yaml:"e2e_queries,omitempty"`
	Tags              []string `yaml:"tags,omitempty"`
}

// NewProvider creates a Network Path Remote Configuration provider.
func NewProvider() *Provider {
	configChanges := make(chan integration.ConfigChanges, 10)
	configChanges <- integration.ConfigChanges{}
	return &Provider{
		configChanges: configChanges,
		shutdownCh:    make(chan struct{}),
		activeByPath:  make(map[string][]integration.Config),
		configErrors:  make(map[string]types.ErrorMsgSet),
	}
}

// String returns the provider name.
func (p *Provider) String() string {
	return names.NetworkPathRemoteConfig
}

// Stream sends Network Path config changes to Autodiscovery.
func (p *Provider) Stream(ctx context.Context) <-chan integration.ConfigChanges {
	go func() {
		<-ctx.Done()
		p.close()
	}()
	return p.configChanges
}

func (p *Provider) close() {
	p.closeOnce.Do(func() {
		close(p.shutdownCh)

		p.stateMutex.Lock()
		defer p.stateMutex.Unlock()
		p.closed = true
		close(p.configChanges)
	})
}

// GetConfigErrors returns configuration errors indexed by RC config path.
func (p *Provider) GetConfigErrors() map[string]types.ErrorMsgSet {
	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()

	errorsByPath := make(map[string]types.ErrorMsgSet, len(p.configErrors))
	maps.Copy(errorsByPath, p.configErrors)
	return errorsByPath
}

// Update handles NETWORK_PATH Remote Configuration snapshots.
func (p *Provider) Update(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	p.stateMutex.Lock()

	changes := integration.ConfigChanges{}
	seenPaths := make(map[string]struct{}, len(updates))

	for path, rawConfig := range updates {
		// Update receives the full NETWORK_PATH snapshot. Paths not marked as seen here
		// are treated as deleted after the snapshot has been processed.
		seenPaths[path] = struct{}{}

		configs, err := parseConfig(rawConfig.Config)
		if err != nil {
			// Keep the last valid configs active when a replacement payload is invalid.
			log.Warnf("Skipping invalid NETWORK_PATH update %s: %v", path, err)
			p.configErrors[path] = errorSet(err)
			applyStateCallback(path, state.ApplyStatus{
				State: state.ApplyStateError,
				Error: err.Error(),
			})
			continue
		}

		delete(p.configErrors, path)
		applyStateCallback(path, state.ApplyStatus{State: state.ApplyStateAcknowledged})

		current := p.activeByPath[path]
		// Keep no-op snapshots from emitting unschedule/schedule churn before downstream dedupe.
		if sameConfigs(current, configs) {
			continue
		}

		// A valid snapshot replaces the whole config set for this RC path.
		changes.Unschedule = append(changes.Unschedule, current...)
		p.activeByPath[path] = configs
		changes.Schedule = append(changes.Schedule, configs...)
	}

	for path, current := range p.activeByPath {
		if _, found := seenPaths[path]; found {
			continue
		}
		// Active paths missing from the snapshot were deleted from RC.
		changes.Unschedule = append(changes.Unschedule, current...)
		delete(p.activeByPath, path)
		delete(p.configErrors, path)
	}
	for path := range p.configErrors {
		if _, found := seenPaths[path]; found {
			continue
		}
		// Drop stale errors for deleted paths that never had active configs.
		delete(p.configErrors, path)
	}

	p.stateMutex.Unlock()
	p.sendChanges(changes)
}

func (p *Provider) sendChanges(changes integration.ConfigChanges) {
	if changes.IsEmpty() {
		return
	}

	p.stateMutex.RLock()
	defer p.stateMutex.RUnlock()
	if p.closed {
		return
	}
	select {
	case p.configChanges <- changes:
	case <-p.shutdownCh:
	}
}

func parseConfig(raw []byte) ([]integration.Config, error) {
	var envelope remoteConfigEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("invalid Network Path config: %w", err)
	}

	if envelope.Type == "" {
		return nil, errors.New("invalid Network Path config: type is required")
	}
	if envelope.Type == dynamicType {
		return nil, errors.New("unsupported Network Path config type \"dynamic\": dynamic Network Path Remote Configuration is not implemented")
	}
	if envelope.Type != scheduledType {
		return nil, fmt.Errorf("unsupported Network Path config type %q", envelope.Type)
	}

	testConfigID := strings.TrimSpace(envelope.TestConfigID)
	if testConfigID == "" {
		return nil, errors.New("invalid Network Path config: test_config_id is required")
	}
	if envelope.Config == nil {
		return nil, errors.New("invalid Network Path config: config must be provided")
	}
	if envelope.Config.Tests == nil {
		return nil, errors.New("invalid Network Path config: config.tests must be provided")
	}
	if len(envelope.Config.Tests) == 0 {
		return nil, errors.New("invalid Network Path config: config.tests must contain at least one item")
	}

	configs := make([]integration.Config, 0, len(envelope.Config.Tests))
	for i, endpoint := range envelope.Config.Tests {
		instance, err := translateEndpoint(testConfigID, endpoint)
		if err != nil {
			return nil, fmt.Errorf("invalid Network Path config at tests[%d]: %w", i, err)
		}

		instanceYAML, err := yaml.Marshal(instance)
		if err != nil {
			return nil, fmt.Errorf("invalid Network Path config at tests[%d]: %w", i, err)
		}

		configs = append(configs, integration.Config{
			Name:      networkpathcheck.CheckName,
			Instances: []integration.Data{integration.Data(instanceYAML)},
			Source:    configSource,
		})
	}

	return configs, nil
}

func translateEndpoint(testConfigID string, endpoint endpointConfig) (networkPathInstanceConfig, error) {
	hostname := strings.TrimSpace(endpoint.Hostname)
	if hostname == "" {
		return networkPathInstanceConfig{}, errors.New("hostname is required")
	}

	instance := networkPathInstanceConfig{
		TestConfigID:          testConfigID,
		Hostname:              hostname,
		SourceService:         endpoint.SourceService,
		DestinationService:    endpoint.DestinationService,
		TracerouteQueries:     endpoint.TracerouteQueries,
		E2eQueries:            endpoint.E2eQueries,
		Tags:                  endpoint.Tags,
		Timeout:               endpoint.TimeoutMS,
		MinCollectionInterval: endpoint.IntervalSec,
	}

	if endpoint.Port != nil {
		if *endpoint.Port < 1 || *endpoint.Port > 65535 {
			return networkPathInstanceConfig{}, errors.New("port must be between 1 and 65535")
		}
		port := uint16(*endpoint.Port)
		instance.Port = &port
	}

	if endpoint.Protocol != nil {
		protocol := payload.Protocol(strings.ToUpper(strings.TrimSpace(*endpoint.Protocol)))
		switch protocol {
		case payload.ProtocolTCP, payload.ProtocolUDP, payload.ProtocolICMP:
			instance.Protocol = string(protocol)
		default:
			return networkPathInstanceConfig{}, fmt.Errorf("unsupported protocol %q", *endpoint.Protocol)
		}
	}

	if endpoint.MaxTTL != nil {
		if *endpoint.MaxTTL < 1 || *endpoint.MaxTTL > 255 {
			return networkPathInstanceConfig{}, errors.New("max_ttl must be between 1 and 255")
		}
		maxTTL := uint8(*endpoint.MaxTTL)
		instance.MaxTTL = &maxTTL
	}

	if endpoint.TimeoutMS != nil && *endpoint.TimeoutMS <= 0 {
		return networkPathInstanceConfig{}, errors.New("timeout_ms must be > 0")
	}
	if endpoint.IntervalSec != nil && *endpoint.IntervalSec <= 0 {
		return networkPathInstanceConfig{}, errors.New("interval_sec must be > 0")
	}

	if endpoint.TCPMethod != nil {
		method := payload.MakeTCPMethod(strings.TrimSpace(*endpoint.TCPMethod))
		switch method {
		case payload.TCPConfigSYN, payload.TCPConfigSACK, payload.TCPConfigPreferSACK, payload.TCPConfigSYNSocket:
			instance.TCPMethod = string(method)
		default:
			return networkPathInstanceConfig{}, fmt.Errorf("unsupported tcp_method %q", *endpoint.TCPMethod)
		}
	}

	if endpoint.TracerouteQueries != nil && *endpoint.TracerouteQueries <= 0 {
		return networkPathInstanceConfig{}, errors.New("traceroute_queries must be > 0")
	}
	if endpoint.E2eQueries != nil && *endpoint.E2eQueries <= 0 {
		return networkPathInstanceConfig{}, errors.New("e2e_queries must be > 0")
	}

	return instance, nil
}

func sameConfigs(a, b []integration.Config) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Source != b[i].Source ||
			a[i].FastDigest() != b[i].FastDigest() {
			return false
		}
	}
	return true
}

func errorSet(err error) types.ErrorMsgSet {
	return types.ErrorMsgSet{err.Error(): struct{}{}}
}
