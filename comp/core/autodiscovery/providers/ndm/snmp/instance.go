// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package snmp

import (
	"fmt"

	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

// keyConfig is the value of the "snmp" key of the NDM Remote Configuration
// document. The JSON names are the backend contract and must not be renamed.
type keyConfig struct {
	InitConfig initConfig         `json:"init_config"`
	Instances  []documentInstance `json:"instances"`
}

// initConfig is the document's init_config block, shared by every instance.
type initConfig struct {
	Namespace             string                            `json:"namespace"`
	DeviceTagsSource      string                            `json:"device_tags_source"`
	OIDBatchSize          int                               `json:"oid_batch_size"`
	BulkMaxRepetitions    int                               `json:"bulk_max_repetitions"`
	MinCollectionInterval int                               `json:"min_collection_interval"`
	UseRCProfiles         *bool                             `json:"use_remote_config_profiles"`
	CollectTopology       *bool                             `json:"collect_topology"`
	CollectVPN            *bool                             `json:"collect_vpn"`
	GlobalMetrics         []profiledefinition.MetricsConfig `json:"global_metrics"`
}

// documentInstance is one device of the document's instances list.
type documentInstance struct {
	IPAddress             string                              `json:"ip_address"`
	CredID                string                              `json:"cred_id"`
	Port                  int                                 `json:"port"`
	TimeoutSec            int                                 `json:"timeout_sec"`
	Retries               int                                 `json:"retries"`
	OIDBatchSize          int                                 `json:"oid_batch_size"`
	BulkMaxRepetitions    int                                 `json:"bulk_max_repetitions"`
	MinCollectionInterval int                                 `json:"min_collection_interval"`
	Profile               string                              `json:"profile"`
	UseRCProfiles         *bool                               `json:"use_remote_config_profiles"`
	UseGlobalMetrics      *bool                               `json:"use_global_metrics"`
	CollectTopology       *bool                               `json:"collect_topology"`
	CollectVPN            *bool                               `json:"collect_vpn"`
	Namespace             string                              `json:"namespace"`
	Tags                  []string                            `json:"tags"`
	DeviceTagsSource      string                              `json:"device_tags_source"`
	Metrics               []profiledefinition.MetricsConfig   `json:"metrics"`
	MetricTags            []profiledefinition.MetricTagConfig `json:"metric_tags"`
	InterfaceConfigs      []snmpintegration.InterfaceConfig   `json:"interface_configs"`
}

// checkInitConfig is the init config handed to the snmp check. The yaml names
// come from pkg/collector/corechecks/snmp/internal/checkconfig.
type checkInitConfig struct {
	Namespace             string                            `yaml:"namespace,omitempty"`
	DeviceTagsSource      string                            `yaml:"device_tags_source,omitempty"`
	OIDBatchSize          int                               `yaml:"oid_batch_size,omitempty"`
	BulkMaxRepetitions    int                               `yaml:"bulk_max_repetitions,omitempty"`
	MinCollectionInterval int                               `yaml:"min_collection_interval,omitempty"`
	UseRCProfiles         *bool                             `yaml:"use_remote_config_profiles,omitempty"`
	CollectTopology       *bool                             `yaml:"collect_topology,omitempty"`
	CollectVPN            *bool                             `yaml:"collect_vpn,omitempty"`
	GlobalMetrics         []profiledefinition.MetricsConfig `yaml:"global_metrics,omitempty"`
	Ping                  *checkPingBlock                   `yaml:"ping,omitempty"`
}

// checkPingBlock is the check's ping block. Its interval and timeout are
// milliseconds, which is what the document's _ms names carry.
type checkPingBlock struct {
	Enabled  *bool                `yaml:"enabled,omitempty"`
	Count    int                  `yaml:"count,omitempty"`
	Interval int                  `yaml:"interval,omitempty"`
	Timeout  int                  `yaml:"timeout,omitempty"`
	Linux    *checkPingLinuxBlock `yaml:"linux,omitempty"`
}

type checkPingLinuxBlock struct {
	UseRawSocket *bool `yaml:"use_raw_socket,omitempty"`
}

// checkInstance is one instance handed to the snmp check. Every field the
// document leaves unset is omitted so the check applies its own default.
type checkInstance struct {
	IPAddress             string                              `yaml:"ip_address"`
	SNMPVersion           string                              `yaml:"snmp_version"`
	CommunityString       string                              `yaml:"community_string,omitempty"`
	User                  string                              `yaml:"user,omitempty"`
	AuthProtocol          string                              `yaml:"authProtocol,omitempty"`
	AuthKey               string                              `yaml:"authKey,omitempty"`
	PrivProtocol          string                              `yaml:"privProtocol,omitempty"`
	PrivKey               string                              `yaml:"privKey,omitempty"`
	ContextName           string                              `yaml:"context_name,omitempty"`
	ContextEngineID       string                              `yaml:"context_engine_id,omitempty"`
	Port                  int                                 `yaml:"port,omitempty"`
	Timeout               int                                 `yaml:"timeout,omitempty"`
	Retries               int                                 `yaml:"retries,omitempty"`
	OIDBatchSize          int                                 `yaml:"oid_batch_size,omitempty"`
	BulkMaxRepetitions    int                                 `yaml:"bulk_max_repetitions,omitempty"`
	MinCollectionInterval int                                 `yaml:"min_collection_interval,omitempty"`
	Profile               string                              `yaml:"profile,omitempty"`
	UseRCProfiles         *bool                               `yaml:"use_remote_config_profiles,omitempty"`
	UseGlobalMetrics      *bool                               `yaml:"use_global_metrics,omitempty"`
	CollectTopology       *bool                               `yaml:"collect_topology,omitempty"`
	CollectVPN            *bool                               `yaml:"collect_vpn,omitempty"`
	Namespace             string                              `yaml:"namespace,omitempty"`
	Tags                  []string                            `yaml:"tags,omitempty"`
	DeviceTagsSource      string                              `yaml:"device_tags_source,omitempty"`
	Metrics               []profiledefinition.MetricsConfig   `yaml:"metrics,omitempty"`
	MetricTags            []profiledefinition.MetricTagConfig `yaml:"metric_tags,omitempty"`
	Ping                  *checkPingBlock                     `yaml:"ping,omitempty"`
	InterfaceConfigs      []snmpintegration.InterfaceConfig   `yaml:"interface_configs,omitempty"`
}

// renderInitConfig turns the document's init_config into the check's, with
// the ping options every pinged device of the document inherits.
func renderInitConfig(ic initConfig, sharedPing *pingOptions) (integration.Data, error) {
	body, err := yaml.Marshal(checkInitConfig{
		Namespace:             ic.Namespace,
		DeviceTagsSource:      ic.DeviceTagsSource,
		OIDBatchSize:          ic.OIDBatchSize,
		BulkMaxRepetitions:    ic.BulkMaxRepetitions,
		MinCollectionInterval: ic.MinCollectionInterval,
		UseRCProfiles:         ic.UseRCProfiles,
		CollectTopology:       ic.CollectTopology,
		CollectVPN:            ic.CollectVPN,
		GlobalMetrics:         ic.GlobalMetrics,
		Ping:                  renderSharedPing(sharedPing),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to render the init config: %w", err)
	}
	return integration.Data(body), nil
}

// renderInstance joins a document instance with its resolved credential into
// one check instance. ping carries the device's ping options when the
// document's ping section lists it, and is nil otherwise.
func renderInstance(in documentInstance, c credential, ping *pingOptions) (integration.Data, error) {
	body, err := yaml.Marshal(checkInstance{
		IPAddress:             in.IPAddress,
		SNMPVersion:           c.SNMPVersion,
		CommunityString:       c.CommunityString,
		User:                  c.User,
		AuthProtocol:          c.AuthProtocol,
		AuthKey:               c.AuthKey,
		PrivProtocol:          c.PrivProtocol,
		PrivKey:               c.PrivKey,
		ContextName:           c.ContextName,
		ContextEngineID:       c.ContextEngineID,
		Port:                  in.Port,
		Timeout:               in.TimeoutSec,
		Retries:               in.Retries,
		OIDBatchSize:          in.OIDBatchSize,
		BulkMaxRepetitions:    in.BulkMaxRepetitions,
		MinCollectionInterval: in.MinCollectionInterval,
		Profile:               in.Profile,
		UseRCProfiles:         in.UseRCProfiles,
		UseGlobalMetrics:      in.UseGlobalMetrics,
		CollectTopology:       in.CollectTopology,
		CollectVPN:            in.CollectVPN,
		Namespace:             in.Namespace,
		Tags:                  in.Tags,
		DeviceTagsSource:      in.DeviceTagsSource,
		Metrics:               in.Metrics,
		MetricTags:            in.MetricTags,
		Ping:                  renderDevicePing(ping),
		InterfaceConfigs:      in.InterfaceConfigs,
	})
	if err != nil {
		// The marshalled body can hold a credential value, so it stays out of the error.
		return nil, fmt.Errorf("failed to render the instance for %s", in.IPAddress)
	}
	return integration.Data(body), nil
}

// renderSharedPing turns the ping section's init_config into the check's ping
// block. It carries no enabled flag: only the devices the section lists are
// pinged, and each one enables ping on its own instance.
func renderSharedPing(p *pingOptions) *checkPingBlock {
	if p == nil {
		return nil
	}
	block := pingBlock(*p)
	if block == (checkPingBlock{}) {
		return nil
	}
	return &block
}

// renderDevicePing turns one device's ping options into the check's ping
// block, enabling ping on it. A nil p means the document does not ping the
// device, which leaves the check's own default of disabled.
func renderDevicePing(p *pingOptions) *checkPingBlock {
	if p == nil {
		return nil
	}
	block := pingBlock(*p)
	block.Enabled = &pingEnabled
	return &block
}

// pingEnabled is the value every pinged device's instance points enabled at.
var pingEnabled = true

func pingBlock(p pingOptions) checkPingBlock {
	block := checkPingBlock{
		Count:    p.Count,
		Interval: p.IntervalMS,
		Timeout:  p.TimeoutMS,
	}
	if p.Linux.UseRawSocket != nil {
		block.Linux = &checkPingLinuxBlock{UseRawSocket: p.Linux.UseRawSocket}
	}
	return block
}
