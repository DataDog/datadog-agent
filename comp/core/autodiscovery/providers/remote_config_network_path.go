// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/networkpath"
	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	networkPathDebugPOCMarker = "network_path_scheduled_tests"
	networkPathScheduledType  = "scheduled"
)

var reservedNetworkPathRCTagPrefixes = []string{
	"network_path.test_id:",
	"network_path.config_source:",
	"network_path.rc_product:",
	"network_path.rc_config_id:",
	"network_path.rc_config_version:",
}

// NetworkPathRemoteConfigProvider receives scheduled Network Path test
// configurations from the Remote Config DEBUG product.
type NetworkPathRemoteConfigProvider struct {
	configErrors map[string]types.ErrorMsgSet
	configCache  map[string]integration.Config
	mu           sync.RWMutex
	upToDate     bool
}

type networkPathDebugConfigMarker struct {
	POC string `json:"poc"`
}

type networkPathDebugScheduledConfig struct {
	POC     string                       `json:"poc"`
	Type    string                       `json:"type"`
	Configs []networkPathDebugTestConfig `json:"configs"`
}

type networkPathDebugTestConfig struct {
	TestID             *string  `json:"test_id,omitempty"`
	Hostname           string   `json:"hostname"`
	Port               *int     `json:"port,omitempty"`
	Protocol           *string  `json:"protocol,omitempty"`
	MaxTTL             *int     `json:"max_ttl,omitempty"`
	TimeoutMS          *int     `json:"timeout_ms,omitempty"`
	IntervalSec        *int     `json:"interval_sec,omitempty"`
	SourceService      string   `json:"source_service,omitempty"`
	DestinationService string   `json:"destination_service,omitempty"`
	TCPMethod          *string  `json:"tcp_method,omitempty"`
	TracerouteQueries  *int     `json:"traceroute_queries,omitempty"`
	E2eQueries         *int     `json:"e2e_queries,omitempty"`
	Tags               []string `json:"tags,omitempty"`
}

type networkPathInstanceConfig struct {
	Hostname              string   `yaml:"hostname"`
	Port                  *int     `yaml:"port,omitempty"`
	Protocol              string   `yaml:"protocol,omitempty"`
	MaxTTL                *int     `yaml:"max_ttl,omitempty"`
	Timeout               *int     `yaml:"timeout,omitempty"`
	MinCollectionInterval *int     `yaml:"min_collection_interval,omitempty"`
	SourceService         string   `yaml:"source_service,omitempty"`
	DestinationService    string   `yaml:"destination_service,omitempty"`
	TCPMethod             string   `yaml:"tcp_method,omitempty"`
	TracerouteQueries     *int     `yaml:"traceroute_queries,omitempty"`
	E2eQueries            *int     `yaml:"e2e_queries,omitempty"`
	Tags                  []string `yaml:"tags,omitempty"`
}

// NewNetworkPathRemoteConfigProvider creates a new
// NetworkPathRemoteConfigProvider.
func NewNetworkPathRemoteConfigProvider() *NetworkPathRemoteConfigProvider {
	return &NetworkPathRemoteConfigProvider{
		configErrors: make(map[string]types.ErrorMsgSet),
		configCache:  make(map[string]integration.Config),
		upToDate:     false,
	}
}

// Collect retrieves Network Path integrations from remote-config.
func (rc *NetworkPathRemoteConfigProvider) Collect(_ context.Context) ([]integration.Config, error) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.upToDate = true

	integrationList := []integration.Config{}
	for _, intg := range rc.configCache {
		integrationList = append(integrationList, intg)
	}

	return integrationList, nil
}

// IsUpToDate allows autodiscovery to cache configs as long as no changes are
// detected in remote-config.
func (rc *NetworkPathRemoteConfigProvider) IsUpToDate(_ context.Context) (bool, error) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	return rc.upToDate, nil
}

// String returns a string representation of the provider.
func (rc *NetworkPathRemoteConfigProvider) String() string {
	return names.NetworkPathRemoteConfig
}

// GetConfigErrors returns a map of configuration errors for each configuration path.
func (rc *NetworkPathRemoteConfigProvider) GetConfigErrors() map[string]types.ErrorMsgSet {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	errors := make(map[string]types.ErrorMsgSet, len(rc.configErrors))
	maps.Copy(errors, rc.configErrors)

	return errors
}

// ScheduleCallback handles DEBUG remote-config updates for the Network Path
// scheduled tests POC.
func (rc *NetworkPathRemoteConfigProvider) ScheduleCallback(updates map[string]state.RawConfig, applyStateCallback func(string, state.ApplyStatus)) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	newCache := make(map[string]integration.Config, len(updates))
	newErrors := make(map[string]types.ErrorMsgSet)

	for cfgPath, rawConfig := range updates {
		isMarked, markerErr := isNetworkPathDebugConfig(rawConfig.Config)
		if markerErr != nil {
			if existing, found := rc.configCache[cfgPath]; found {
				newCache[cfgPath] = existing
				recordNetworkPathRCError(newErrors, cfgPath, markerErr, applyStateCallback)
			}
			continue
		}
		if !isMarked {
			continue
		}

		config, err := parseNetworkPathDebugConfig(cfgPath, rawConfig)
		if err != nil {
			if existing, found := rc.configCache[cfgPath]; found {
				newCache[cfgPath] = existing
			}
			recordNetworkPathRCError(newErrors, cfgPath, err, applyStateCallback)
			continue
		}

		if len(config.Instances) > 0 {
			newCache[cfgPath] = config
		}
		applyStateCallback(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
	}

	rc.configCache = newCache
	rc.configErrors = newErrors
	rc.upToDate = false
}

func isNetworkPathDebugConfig(raw []byte) (bool, error) {
	var marker networkPathDebugConfigMarker
	if err := json.Unmarshal(raw, &marker); err != nil {
		return false, err
	}

	return marker.POC == networkPathDebugPOCMarker, nil
}

func parseNetworkPathDebugConfig(cfgPath string, rawConfig state.RawConfig) (integration.Config, error) {
	var scheduled networkPathDebugScheduledConfig
	decoder := json.NewDecoder(bytes.NewReader(rawConfig.Config))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&scheduled); err != nil {
		return integration.Config{}, fmt.Errorf("invalid Network Path DEBUG config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return integration.Config{}, errors.New("invalid Network Path DEBUG config: trailing JSON tokens")
	}

	if scheduled.POC != networkPathDebugPOCMarker {
		return integration.Config{}, fmt.Errorf("invalid Network Path DEBUG config: unexpected poc %q", scheduled.POC)
	}
	if scheduled.Type != networkPathScheduledType {
		return integration.Config{}, fmt.Errorf("unsupported Network Path DEBUG config type %q", scheduled.Type)
	}
	if scheduled.Configs == nil {
		return integration.Config{}, errors.New("invalid Network Path DEBUG config: configs must be provided")
	}

	provenanceTags := networkPathRCProvenanceTags(cfgPath, rawConfig.Metadata)
	instances := make([]integration.Data, 0, len(scheduled.Configs))
	for i, cfg := range scheduled.Configs {
		instance, err := translateNetworkPathDebugTestConfig(cfg, provenanceTags)
		if err != nil {
			return integration.Config{}, fmt.Errorf("invalid Network Path DEBUG config at configs[%d]: %w", i, err)
		}
		instanceYAML, err := yaml.Marshal(instance)
		if err != nil {
			return integration.Config{}, fmt.Errorf("invalid Network Path DEBUG config at configs[%d]: %w", i, err)
		}
		instances = append(instances, integration.Data(instanceYAML))
	}

	return integration.Config{
		Name:      networkpath.CheckName,
		Instances: instances,
		Source:    networkPathRCSource(cfgPath, rawConfig.Metadata),
	}, nil
}

func translateNetworkPathDebugTestConfig(cfg networkPathDebugTestConfig, provenanceTags []string) (networkPathInstanceConfig, error) {
	if strings.TrimSpace(cfg.Hostname) == "" {
		return networkPathInstanceConfig{}, errors.New("hostname is required")
	}
	if cfg.Port != nil && (*cfg.Port < 1 || *cfg.Port > 65535) {
		return networkPathInstanceConfig{}, errors.New("port must be between 1 and 65535")
	}
	if cfg.MaxTTL != nil && (*cfg.MaxTTL < 1 || *cfg.MaxTTL > 255) {
		return networkPathInstanceConfig{}, errors.New("max_ttl must be between 1 and 255")
	}
	if cfg.TimeoutMS != nil && *cfg.TimeoutMS <= 0 {
		return networkPathInstanceConfig{}, errors.New("timeout_ms must be > 0")
	}
	if cfg.IntervalSec != nil && *cfg.IntervalSec <= 0 {
		return networkPathInstanceConfig{}, errors.New("interval_sec must be > 0")
	}
	if cfg.TracerouteQueries != nil && *cfg.TracerouteQueries <= 0 {
		return networkPathInstanceConfig{}, errors.New("traceroute_queries must be > 0")
	}
	if cfg.E2eQueries != nil && *cfg.E2eQueries <= 0 {
		return networkPathInstanceConfig{}, errors.New("e2e_queries must be > 0")
	}

	var protocol string
	if cfg.Protocol != nil {
		protocol = strings.ToUpper(*cfg.Protocol)
		switch payload.Protocol(protocol) {
		case payload.ProtocolTCP, payload.ProtocolUDP, payload.ProtocolICMP:
		default:
			return networkPathInstanceConfig{}, errors.New("protocol must be one of TCP, UDP, or ICMP")
		}
	}

	var tcpMethod string
	if cfg.TCPMethod != nil {
		tcpMethod = strings.ToLower(*cfg.TCPMethod)
		switch payload.TCPMethod(tcpMethod) {
		case payload.TCPConfigSYN, payload.TCPConfigSACK, payload.TCPConfigPreferSACK, payload.TCPConfigSYNSocket:
		default:
			return networkPathInstanceConfig{}, errors.New("tcp_method must be one of syn, sack, prefer_sack, or syn_socket")
		}
	}

	tags := slices.Clone(cfg.Tags)
	for _, tag := range tags {
		if hasReservedNetworkPathRCTagPrefix(tag) {
			return networkPathInstanceConfig{}, fmt.Errorf("tag %q uses a reserved Network Path RC prefix", tag)
		}
	}
	if cfg.TestID != nil {
		testID := strings.TrimSpace(*cfg.TestID)
		if testID == "" {
			return networkPathInstanceConfig{}, errors.New("test_id must not be empty")
		}
		if strings.ContainsAny(testID, ",\n\r") {
			return networkPathInstanceConfig{}, errors.New("test_id must not contain commas or newlines")
		}
		tags = append(tags, "network_path.test_id:"+testID)
	}
	tags = append(tags, provenanceTags...)

	return networkPathInstanceConfig{
		Hostname:              cfg.Hostname,
		Port:                  cfg.Port,
		Protocol:              protocol,
		MaxTTL:                cfg.MaxTTL,
		Timeout:               cfg.TimeoutMS,
		MinCollectionInterval: cfg.IntervalSec,
		SourceService:         cfg.SourceService,
		DestinationService:    cfg.DestinationService,
		TCPMethod:             tcpMethod,
		TracerouteQueries:     cfg.TracerouteQueries,
		E2eQueries:            cfg.E2eQueries,
		Tags:                  tags,
	}, nil
}

func networkPathRCProvenanceTags(cfgPath string, metadata state.Metadata) []string {
	configID := networkPathRCConfigID(cfgPath, metadata)
	tags := []string{
		"network_path.config_source:remote_config",
		"network_path.rc_product:debug",
		"network_path.rc_config_id:" + sanitizeNetworkPathRCTagValue(configID),
	}
	if metadata.Version > 0 {
		tags = append(tags, fmt.Sprintf("network_path.rc_config_version:%d", metadata.Version))
	}

	return tags
}

func networkPathRCSource(cfgPath string, metadata state.Metadata) string {
	configName := metadata.Name
	if configName == "" {
		configName = "unnamed"
	}

	return fmt.Sprintf("remote_config_debug/network_path/%s/%s",
		sanitizeNetworkPathRCSourceSegment(networkPathRCConfigID(cfgPath, metadata)),
		sanitizeNetworkPathRCSourceSegment(configName),
	)
}

func networkPathRCConfigID(cfgPath string, metadata state.Metadata) string {
	if metadata.ID != "" {
		return metadata.ID
	}

	return cfgPath
}

func hasReservedNetworkPathRCTagPrefix(tag string) bool {
	for _, prefix := range reservedNetworkPathRCTagPrefixes {
		if strings.HasPrefix(tag, prefix) {
			return true
		}
	}
	return false
}

func sanitizeNetworkPathRCTagValue(value string) string {
	value = strings.TrimSpace(value)
	replacer := strings.NewReplacer(",", "_", "\n", "_", "\r", "_", " ", "_")
	return replacer.Replace(value)
}

func sanitizeNetworkPathRCSourceSegment(value string) string {
	value = sanitizeNetworkPathRCTagValue(value)
	replacer := strings.NewReplacer("/", "_")
	return replacer.Replace(value)
}

func recordNetworkPathRCError(errors map[string]types.ErrorMsgSet, cfgPath string, err error, applyStateCallback func(string, state.ApplyStatus)) {
	errors[cfgPath] = types.ErrorMsgSet{err.Error(): struct{}{}}
	applyStateCallback(cfgPath, state.ApplyStatus{
		State: state.ApplyStateError,
		Error: err.Error(),
	})
}

var _ types.CollectingConfigProvider = (*NetworkPathRemoteConfigProvider)(nil)
var _ types.ConfigProvider = (*NetworkPathRemoteConfigProvider)(nil)

// NetworkPathRemoteConfigProduct is the RC product consumed by this provider.
const NetworkPathRemoteConfigProduct = data.ProductDebug
