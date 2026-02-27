// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package snmp

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"sort"
	"strconv"
	"time"

	"github.com/gosnmp/gosnmp"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/structure"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpintegration"
)

const (
	defaultPort = 161
)

// ListenerConfig holds global configuration for SNMP discovery
type ListenerConfig struct {
	Workers                 int                        `mapstructure:"workers"`
	DiscoveryInterval       int                        `mapstructure:"discovery_interval"`
	AllowedFailures         int                        `mapstructure:"discovery_allowed_failures"`
	Loader                  string                     `mapstructure:"loader"`
	CollectDeviceMetadata   bool                       `mapstructure:"collect_device_metadata"`
	CollectTopology         bool                       `mapstructure:"collect_topology"`
	CollectVPN              bool                       `mapstructure:"collect_vpn"`
	MinCollectionInterval   uint                       `mapstructure:"min_collection_interval"`
	Namespace               string                     `mapstructure:"namespace"`
	UseDeviceISAsHostname   bool                       `mapstructure:"use_device_id_as_hostname"`
	PingConfig              snmpintegration.PingConfig `mapstructure:"ping"`
	Deduplicate             bool                       `mapstructure:"use_deduplication"`
	UseRemoteConfigProfiles bool                       `mapstructure:"use_remote_config_profiles"`
	OidBatchSize            int                        `mapstructure:"oid_batch_size"`
	Timeout                 int                        `mapstructure:"timeout"`
	Retries                 int                        `mapstructure:"retries"`

	// legacy
	AllowedFailuresLegacy int `mapstructure:"allowed_failures"`

	Configs []Config

	// DON'T USE. This is only used to read the raw array from datadog.yaml
	UnmarshalledConfigs []UnmarshalledConfig `mapstructure:"configs"`
}

// UnmarshalledConfig is used to read each item of the array in datadog.yaml
type UnmarshalledConfig struct {
	ADIdentifier          string                                       `mapstructure:"ad_identifier"`
	AuthKey               string                                       `mapstructure:"authKey"`
	AuthProtocol          string                                       `mapstructure:"authProtocol"`
	Authentications       []Authentication                             `mapstructure:"authentications"`
	CollectDeviceMetadata *bool                                        `mapstructure:"collect_device_metadata"`
	CollectTopology       *bool                                        `mapstructure:"collect_topology"`
	CollectVPN            *bool                                        `mapstructure:"collect_vpn"`
	Community             string                                       `mapstructure:"community_string"`
	ContextEngineID       string                                       `mapstructure:"context_engine_id"`
	ContextName           string                                       `mapstructure:"context_name"`
	IgnoredIPAddresses    []string                                     `mapstructure:"ignored_ip_addresses"`
	InterfaceConfigs      map[string][]snmpintegration.InterfaceConfig `mapstructure:"interface_configs"`
	Loader                string                                       `mapstructure:"loader"`
	MinCollectionInterval uint                                         `mapstructure:"min_collection_interval"`
	Namespace             string                                       `mapstructure:"namespace"`
	Network               string                                       `mapstructure:"network_address"`
	OidBatchSize          int                                          `mapstructure:"oid_batch_size"`
	PingConfig            snmpintegration.PingConfig                   `mapstructure:"ping"`
	Port                  uint16                                       `mapstructure:"port"`
	PrivKey               string                                       `mapstructure:"privKey"`
	PrivProtocol          string                                       `mapstructure:"privProtocol"`
	Retries               int                                          `mapstructure:"retries"`
	Tags                  []string                                     `mapstructure:"tags"`
	Timeout               int                                          `mapstructure:"timeout"`
	UseDeviceIDAsHostname *bool                                        `mapstructure:"use_device_id_as_hostname"`
	User                  string                                       `mapstructure:"user"`
	Version               string                                       `mapstructure:"snmp_version"`

	// Legacy
	NetworkLegacy      string `mapstructure:"network"`
	VersionLegacy      string `mapstructure:"version"`
	CommunityLegacy    string `mapstructure:"community"`
	AuthKeyLegacy      string `mapstructure:"authentication_key"`
	AuthProtocolLegacy string `mapstructure:"authentication_protocol"`
	PrivKeyLegacy      string `mapstructure:"privacy_key"`
	PrivProtocolLegacy string `mapstructure:"privacy_protocol"`
}

// Config holds configuration for a particular subnet
type Config struct {
	ADIdentifier            string
	AuthKey                 string
	AuthProtocol            string
	Authentications         []Authentication
	Community               string
	ContextEngineID         string
	ContextName             string
	Loader                  string
	MinCollectionInterval   uint
	Namespace               string
	Network                 string
	OidBatchSize            int
	Port                    uint16
	PrivKey                 string
	PrivProtocol            string
	Retries                 int
	Tags                    []string
	Timeout                 int
	User                    string
	Version                 string
	CollectDeviceMetadata   bool
	CollectTopology         bool
	CollectVPN              bool
	IgnoredIPAddresses      map[string]bool
	UseDeviceIDAsHostname   bool
	UseRemoteConfigProfiles bool
	PingConfig              snmpintegration.PingConfig

	// InterfaceConfigs is a map of IP to a list of snmpintegration.InterfaceConfig
	InterfaceConfigs map[string][]snmpintegration.InterfaceConfig
}

// Authentication holds SNMP authentication data
type Authentication struct {
	Version         string `mapstructure:"snmp_version"`
	Timeout         int    `mapstructure:"timeout"`
	Retries         int    `mapstructure:"retries"`
	Community       string `mapstructure:"community_string"`
	User            string `mapstructure:"user"`
	AuthKey         string `mapstructure:"authKey"`
	AuthProtocol    string `mapstructure:"authProtocol"`
	PrivKey         string `mapstructure:"privKey"`
	PrivProtocol    string `mapstructure:"privProtocol"`
	ContextEngineID string `mapstructure:"context_engine_id"`
	ContextName     string `mapstructure:"context_name"`
}

type intOrBoolPtr interface {
	*int | *bool
}

// ErrNoConfigGiven is returned when the SNMP listener config was not found
var ErrNoConfigGiven = errors.New("no config given for snmp_listener")

// NewListenerConfig parses configuration and returns a built ListenerConfig
func NewListenerConfig() (ListenerConfig, error) {
	ddcfg := pkgconfigsetup.Datadog()

	var snmpConfig ListenerConfig

	possibleConfigKeys := []string{
		"network_devices.autodiscovery",
		"snmp_listener",
	}

	configKey := ""
	for _, key := range possibleConfigKeys {
		if ddcfg.IsConfigured(key) {
			configKey = key
			break
		}
	}

	if configKey == "" {
		return ListenerConfig{}, ErrNoConfigGiven
	}

	err := structure.UnmarshalKey(ddcfg, configKey, &snmpConfig)
	if err != nil {
		return ListenerConfig{}, err
	}

	if ddcfg.IsConfigured(configKey+".allowed_failures") && !ddcfg.IsConfigured(configKey+".discovery_allowed_failures") {
		snmpConfig.AllowedFailures = snmpConfig.AllowedFailuresLegacy
	}

	if !ddcfg.IsConfigured(configKey+".namespace") && ddcfg.IsConfigured("network_devices.namespace") {
		snmpConfig.Namespace = ddcfg.GetString("network_devices.namespace")
	}

	snmpConfig.Configs = make([]Config, len(snmpConfig.UnmarshalledConfigs))

	// Set the default values and resolve 'computed' fields from raw and legacy
	for i, unmarshalledConfig := range snmpConfig.UnmarshalledConfigs {
		snmpConfig.Configs[i] = Config{
			ADIdentifier:          unmarshalledConfig.ADIdentifier,
			AuthKey:               unmarshalledConfig.AuthKey,
			AuthProtocol:          unmarshalledConfig.AuthProtocol,
			Authentications:       unmarshalledConfig.Authentications,
			Community:             unmarshalledConfig.Community,
			ContextEngineID:       unmarshalledConfig.ContextEngineID,
			ContextName:           unmarshalledConfig.ContextName,
			InterfaceConfigs:      unmarshalledConfig.InterfaceConfigs,
			Loader:                unmarshalledConfig.Loader,
			MinCollectionInterval: unmarshalledConfig.MinCollectionInterval,
			Namespace:             unmarshalledConfig.Namespace,
			Network:               unmarshalledConfig.Network,
			OidBatchSize:          unmarshalledConfig.OidBatchSize,
			PingConfig:            unmarshalledConfig.PingConfig,
			Port:                  unmarshalledConfig.Port,
			PrivKey:               unmarshalledConfig.PrivKey,
			PrivProtocol:          unmarshalledConfig.PrivProtocol,
			Retries:               unmarshalledConfig.Retries,
			Tags:                  unmarshalledConfig.Tags,
			Timeout:               unmarshalledConfig.Timeout,
			User:                  unmarshalledConfig.User,
			Version:               unmarshalledConfig.Version,
		}

		config := &snmpConfig.Configs[i]

		if config.Port == 0 {
			config.Port = defaultPort
		}

		if config.Timeout == 0 {
			config.Timeout = snmpConfig.Timeout
		}

		if config.Retries == 0 {
			config.Retries = snmpConfig.Retries
		}

		if unmarshalledConfig.CollectDeviceMetadata != nil {
			config.CollectDeviceMetadata = *unmarshalledConfig.CollectDeviceMetadata
		} else {
			config.CollectDeviceMetadata = snmpConfig.CollectDeviceMetadata
		}

		if unmarshalledConfig.CollectTopology != nil {
			config.CollectTopology = *unmarshalledConfig.CollectTopology
		} else {
			config.CollectTopology = snmpConfig.CollectTopology
		}

		if unmarshalledConfig.CollectVPN != nil {
			config.CollectVPN = *unmarshalledConfig.CollectVPN
		} else {
			config.CollectVPN = snmpConfig.CollectVPN
		}

		if unmarshalledConfig.UseDeviceIDAsHostname != nil {
			config.UseDeviceIDAsHostname = *unmarshalledConfig.UseDeviceIDAsHostname
		} else {
			config.UseDeviceIDAsHostname = snmpConfig.UseDeviceISAsHostname
		}

		if config.Loader == "" {
			config.Loader = snmpConfig.Loader
		}

		if config.MinCollectionInterval == 0 {
			config.MinCollectionInterval = snmpConfig.MinCollectionInterval
		}

		if config.OidBatchSize == 0 {
			config.OidBatchSize = snmpConfig.OidBatchSize
		}

		config.PingConfig.Enabled = firstNonNil(config.PingConfig.Enabled, snmpConfig.PingConfig.Enabled)
		config.PingConfig.Linux.UseRawSocket = firstNonNil(config.PingConfig.Linux.UseRawSocket, snmpConfig.PingConfig.Linux.UseRawSocket)
		config.PingConfig.Interval = firstNonNil(config.PingConfig.Interval, snmpConfig.PingConfig.Interval)
		config.PingConfig.Timeout = firstNonNil(config.PingConfig.Timeout, snmpConfig.PingConfig.Timeout)
		config.PingConfig.Count = firstNonNil(config.PingConfig.Count, snmpConfig.PingConfig.Count)

		config.Namespace = firstNonEmpty(config.Namespace, snmpConfig.Namespace)
		config.Community = firstNonEmpty(config.Community, unmarshalledConfig.CommunityLegacy)
		config.AuthKey = firstNonEmpty(config.AuthKey, unmarshalledConfig.AuthKeyLegacy)
		config.AuthProtocol = firstNonEmpty(config.AuthProtocol, unmarshalledConfig.AuthProtocolLegacy)
		config.PrivKey = firstNonEmpty(config.PrivKey, unmarshalledConfig.PrivKeyLegacy)
		config.PrivProtocol = firstNonEmpty(config.PrivProtocol, unmarshalledConfig.PrivProtocolLegacy)
		config.Network = firstNonEmpty(config.Network, unmarshalledConfig.NetworkLegacy)
		config.Version = firstNonEmpty(config.Version, unmarshalledConfig.VersionLegacy)

		if config.Community != "" || config.User != "" {
			config.Authentications = append([]Authentication{
				{
					Version:         config.Version,
					Timeout:         config.Timeout,
					Retries:         config.Retries,
					Community:       config.Community,
					User:            config.User,
					AuthKey:         config.AuthKey,
					AuthProtocol:    config.AuthProtocol,
					PrivKey:         config.PrivKey,
					PrivProtocol:    config.PrivProtocol,
					ContextEngineID: config.ContextEngineID,
					ContextName:     config.ContextName,
				},
			}, config.Authentications...)
		}

		for authIndex := range config.Authentications {
			if config.Authentications[authIndex].Timeout == 0 {
				config.Authentications[authIndex].Timeout = config.Timeout
			}
			if config.Authentications[authIndex].Retries == 0 {
				config.Authentications[authIndex].Retries = config.Retries
			}
		}

		config.UseRemoteConfigProfiles = snmpConfig.UseRemoteConfigProfiles

		config.IgnoredIPAddresses = make(map[string]bool, len(unmarshalledConfig.IgnoredIPAddresses))
		for _, ip := range unmarshalledConfig.IgnoredIPAddresses {
			config.IgnoredIPAddresses[ip] = true
		}
	}

	snmpConfig.UnmarshalledConfigs = nil

	return snmpConfig, nil
}

// LegacyDigest returns an hash value representing the data stored in this configuration, minus the network address and authentications
// TODO: Remove support for legacy format when Agent reaches version 7.76+: see https://github.com/DataDog/datadog-agent/pull/39459
func (c *Config) LegacyDigest(address string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(address))                                //nolint:errcheck
	h.Write([]byte(strconv.FormatUint(uint64(c.Port), 10))) //nolint:errcheck

	h.Write([]byte(c.Version))         //nolint:errcheck
	h.Write([]byte(c.Community))       //nolint:errcheck
	h.Write([]byte(c.User))            //nolint:errcheck
	h.Write([]byte(c.AuthKey))         //nolint:errcheck
	h.Write([]byte(c.AuthProtocol))    //nolint:errcheck
	h.Write([]byte(c.PrivKey))         //nolint:errcheck
	h.Write([]byte(c.PrivProtocol))    //nolint:errcheck
	h.Write([]byte(c.ContextEngineID)) //nolint:errcheck
	h.Write([]byte(c.ContextName))     //nolint:errcheck

	h.Write([]byte(c.Loader))    //nolint:errcheck
	h.Write([]byte(c.Namespace)) //nolint:errcheck

	// Sort the addresses to get a stable digest
	addresses := make([]string, 0, len(c.IgnoredIPAddresses))
	for ip := range c.IgnoredIPAddresses {
		addresses = append(addresses, ip)
	}
	sort.Strings(addresses)
	for _, ip := range addresses {
		h.Write([]byte(ip)) //nolint:errcheck
	}

	return strconv.FormatUint(h.Sum64(), 16)
}

// Digest returns an hash value representing the data stored in this configuration, minus the network address
func (c *Config) Digest(address string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(address))                                //nolint:errcheck
	h.Write([]byte(strconv.FormatUint(uint64(c.Port), 10))) //nolint:errcheck

	for _, authentication := range c.Authentications {
		h.Write([]byte(authentication.Version))         //nolint:errcheck
		h.Write([]byte(authentication.Community))       //nolint:errcheck
		h.Write([]byte(authentication.User))            //nolint:errcheck
		h.Write([]byte(authentication.AuthKey))         //nolint:errcheck
		h.Write([]byte(authentication.AuthProtocol))    //nolint:errcheck
		h.Write([]byte(authentication.PrivKey))         //nolint:errcheck
		h.Write([]byte(authentication.PrivProtocol))    //nolint:errcheck
		h.Write([]byte(authentication.ContextEngineID)) //nolint:errcheck
		h.Write([]byte(authentication.ContextName))     //nolint:errcheck
	}

	h.Write([]byte(c.Loader))    //nolint:errcheck
	h.Write([]byte(c.Namespace)) //nolint:errcheck

	// Sort the addresses to get a stable digest
	addresses := make([]string, 0, len(c.IgnoredIPAddresses))
	for ip := range c.IgnoredIPAddresses {
		addresses = append(addresses, ip)
	}
	sort.Strings(addresses)
	for _, ip := range addresses {
		h.Write([]byte(ip)) //nolint:errcheck
	}

	return strconv.FormatUint(h.Sum64(), 16)
}

// IsIPIgnored checks the given IP against IgnoredIPAddresses
func (c *Config) IsIPIgnored(ip net.IP) bool {
	ipString := ip.String()
	_, present := c.IgnoredIPAddresses[ipString]
	return present
}

// BuildSNMPParams returns a valid GoSNMP struct to start making queries
func (a *Authentication) BuildSNMPParams(deviceIP string, port uint16) (*gosnmp.GoSNMP, error) {
	if a.Community == "" && a.User == "" {
		return nil, errors.New("No authentication mechanism specified")
	}

	var version gosnmp.SnmpVersion
	if a.Version == "1" {
		version = gosnmp.Version1
	} else if a.Version == "2" || a.Version == "2c" || a.Version == "2C" || (a.Version == "" && a.Community != "") {
		version = gosnmp.Version2c
	} else if a.Version == "3" || (a.Version == "" && a.User != "") {
		version = gosnmp.Version3
	} else {
		return nil, fmt.Errorf("SNMP version not supported: %s", a.Version)
	}

	authProtocol, err := gosnmplib.GetAuthProtocol(a.AuthProtocol)
	if err != nil {
		return nil, err
	}

	privProtocol, err := gosnmplib.GetPrivProtocol(a.PrivProtocol)
	if err != nil {
		return nil, err
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if a.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if a.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Target:          deviceIP,
		Port:            port,
		Community:       a.Community,
		Transport:       "udp",
		Version:         version,
		Timeout:         time.Duration(a.Timeout) * time.Second,
		Retries:         a.Retries,
		SecurityModel:   gosnmp.UserSecurityModel,
		MsgFlags:        msgFlags,
		ContextEngineID: a.ContextEngineID,
		ContextName:     a.ContextName,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 a.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: a.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        a.PrivKey,
		},
	}, nil
}

func firstNonEmpty(strings ...string) string {
	for index, s := range strings {
		if s != "" || index == len(strings)-1 {
			return s
		}
	}
	return ""
}

func firstNonNil[T intOrBoolPtr](params ...T) T {
	for _, p := range params {
		if p != nil {
			return p
		}
	}

	return nil
}
