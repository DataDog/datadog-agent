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
	defaultPort    = 161
	defaultTimeout = 5
	defaultRetries = 3
)

// ListenerConfig holds global configuration for SNMP discovery
type ListenerConfig struct {
	Workers               int                        `mapstructure:"workers"`
	DiscoveryInterval     int                        `mapstructure:"discovery_interval"`
	AllowedFailures       int                        `mapstructure:"discovery_allowed_failures"`
	Loader                string                     `mapstructure:"loader"`
	CollectDeviceMetadata bool                       `mapstructure:"collect_device_metadata"`
	CollectTopology       bool                       `mapstructure:"collect_topology"`
	CollectVPN            bool                       `mapstructure:"collect_vpn"`
	MinCollectionInterval uint                       `mapstructure:"min_collection_interval"`
	Namespace             string                     `mapstructure:"namespace"`
	UseDeviceISAsHostname bool                       `mapstructure:"use_device_id_as_hostname"`
	Configs               []Config                   `mapstructure:"configs"`
	PingConfig            snmpintegration.PingConfig `mapstructure:"ping"`
	Deduplicate           bool                       `mapstructure:"use_deduplication"`

	// legacy
	AllowedFailuresLegacy int `mapstructure:"allowed_failures"`
}

// Config holds configuration for a particular subnet
type Config struct {
	Network                     string           `mapstructure:"network_address"`
	Port                        uint16           `mapstructure:"port"`
	Version                     string           `mapstructure:"snmp_version"`
	Timeout                     int              `mapstructure:"timeout"`
	Retries                     int              `mapstructure:"retries"`
	OidBatchSize                int              `mapstructure:"oid_batch_size"`
	Community                   string           `mapstructure:"community_string"`
	User                        string           `mapstructure:"user"`
	AuthKey                     string           `mapstructure:"authKey"`
	AuthProtocol                string           `mapstructure:"authProtocol"`
	PrivKey                     string           `mapstructure:"privKey"`
	PrivProtocol                string           `mapstructure:"privProtocol"`
	ContextEngineID             string           `mapstructure:"context_engine_id"`
	ContextName                 string           `mapstructure:"context_name"`
	Authentications             []Authentication `mapstructure:"authentications"`
	IgnoredIPAddresses          map[string]bool  `mapstructure:"ignored_ip_addresses"`
	ADIdentifier                string           `mapstructure:"ad_identifier"`
	Loader                      string           `mapstructure:"loader"`
	CollectDeviceMetadataConfig *bool            `mapstructure:"collect_device_metadata"`
	CollectDeviceMetadata       bool
	CollectTopologyConfig       *bool `mapstructure:"collect_topology"`
	CollectTopology             bool
	CollectVPNConfig            *bool `mapstructure:"collect_vpn"`
	CollectVPN                  bool
	UseDeviceIDAsHostnameConfig *bool `mapstructure:"use_device_id_as_hostname"`
	UseDeviceIDAsHostname       bool
	Namespace                   string   `mapstructure:"namespace"`
	Tags                        []string `mapstructure:"tags"`
	MinCollectionInterval       uint     `mapstructure:"min_collection_interval"`

	// InterfaceConfigs is a map of IP to a list of snmpintegration.InterfaceConfig
	InterfaceConfigs map[string][]snmpintegration.InterfaceConfig `mapstructure:"interface_configs"`

	PingConfig snmpintegration.PingConfig `mapstructure:"ping"`

	// Legacy
	NetworkLegacy      string `mapstructure:"network"`
	VersionLegacy      string `mapstructure:"version"`
	CommunityLegacy    string `mapstructure:"community"`
	AuthKeyLegacy      string `mapstructure:"authentication_key"`
	AuthProtocolLegacy string `mapstructure:"authentication_protocol"`
	PrivKeyLegacy      string `mapstructure:"privacy_key"`
	PrivProtocolLegacy string `mapstructure:"privacy_protocol"`
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
	var snmpConfig ListenerConfig
	// Set defaults before unmarshalling
	snmpConfig.CollectDeviceMetadata = true
	snmpConfig.CollectTopology = true

	ddcfg := pkgconfigsetup.Datadog()
	if ddcfg.IsSet("network_devices.autodiscovery") {
		err := structure.UnmarshalKey(ddcfg, "network_devices.autodiscovery", &snmpConfig, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			return snmpConfig, err
		}
	} else if ddcfg.IsSet("snmp_listener") {
		err := structure.UnmarshalKey(ddcfg, "snmp_listener", &snmpConfig, structure.ImplicitlyConvertArrayToMapSet)
		if err != nil {
			return snmpConfig, err
		}
	} else {
		return snmpConfig, ErrNoConfigGiven
	}

	if snmpConfig.AllowedFailures == 0 && snmpConfig.AllowedFailuresLegacy != 0 {
		snmpConfig.AllowedFailures = snmpConfig.AllowedFailuresLegacy
	}

	// Set the default values, we can't otherwise on an array
	for i := range snmpConfig.Configs {
		// We need to modify the struct in place
		config := &snmpConfig.Configs[i]
		if config.Port == 0 {
			config.Port = defaultPort
		}
		if config.Timeout == 0 {
			config.Timeout = defaultTimeout
		}
		if config.Retries == 0 {
			config.Retries = defaultRetries
		}
		if config.CollectDeviceMetadataConfig != nil {
			config.CollectDeviceMetadata = *config.CollectDeviceMetadataConfig
		} else {
			config.CollectDeviceMetadata = snmpConfig.CollectDeviceMetadata
		}
		if config.CollectTopologyConfig != nil {
			config.CollectTopology = *config.CollectTopologyConfig
		} else {
			config.CollectTopology = snmpConfig.CollectTopology
		}
		if config.CollectVPNConfig != nil {
			config.CollectVPN = *config.CollectVPNConfig
		} else {
			config.CollectVPN = snmpConfig.CollectVPN
		}

		if config.UseDeviceIDAsHostnameConfig != nil {
			config.UseDeviceIDAsHostname = *config.UseDeviceIDAsHostnameConfig
		} else {
			config.UseDeviceIDAsHostname = snmpConfig.UseDeviceISAsHostname
		}

		if config.Loader == "" {
			config.Loader = snmpConfig.Loader
		}

		if config.MinCollectionInterval == 0 {
			config.MinCollectionInterval = snmpConfig.MinCollectionInterval
		}

		// Ping config
		config.PingConfig.Enabled = firstNonNil(config.PingConfig.Enabled, snmpConfig.PingConfig.Enabled)
		config.PingConfig.Linux.UseRawSocket = firstNonNil(config.PingConfig.Linux.UseRawSocket, snmpConfig.PingConfig.Linux.UseRawSocket)
		config.PingConfig.Interval = firstNonNil(config.PingConfig.Interval, snmpConfig.PingConfig.Interval)
		config.PingConfig.Timeout = firstNonNil(config.PingConfig.Timeout, snmpConfig.PingConfig.Timeout)
		config.PingConfig.Count = firstNonNil(config.PingConfig.Count, snmpConfig.PingConfig.Count)

		config.Namespace = firstNonEmpty(config.Namespace, snmpConfig.Namespace, pkgconfigsetup.Datadog().GetString("network_devices.namespace"))
		config.Community = firstNonEmpty(config.Community, config.CommunityLegacy)
		config.AuthKey = firstNonEmpty(config.AuthKey, config.AuthKeyLegacy)
		config.AuthProtocol = firstNonEmpty(config.AuthProtocol, config.AuthProtocolLegacy)
		config.PrivKey = firstNonEmpty(config.PrivKey, config.PrivKeyLegacy)
		config.PrivProtocol = firstNonEmpty(config.PrivProtocol, config.PrivProtocolLegacy)
		config.Network = firstNonEmpty(config.Network, config.NetworkLegacy)
		config.Version = firstNonEmpty(config.Version, config.VersionLegacy)

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
				config.Authentications[authIndex].Timeout = defaultTimeout
			}
			if config.Authentications[authIndex].Retries == 0 {
				config.Authentications[authIndex].Retries = defaultRetries
			}
		}
	}
	return snmpConfig, nil
}

// Digest returns an hash value representing the data stored in this configuration, minus the network address
func (c *Config) Digest(address string) string {
	h := fnv.New64()
	// Hash write never returns an error
	h.Write([]byte(address))                   //nolint:errcheck
	h.Write([]byte(fmt.Sprintf("%d", c.Port))) //nolint:errcheck
	h.Write([]byte(c.Version))                 //nolint:errcheck
	h.Write([]byte(c.Community))               //nolint:errcheck
	h.Write([]byte(c.User))                    //nolint:errcheck
	h.Write([]byte(c.AuthKey))                 //nolint:errcheck
	h.Write([]byte(c.AuthProtocol))            //nolint:errcheck
	h.Write([]byte(c.PrivKey))                 //nolint:errcheck
	h.Write([]byte(c.PrivProtocol))            //nolint:errcheck
	h.Write([]byte(c.ContextEngineID))         //nolint:errcheck
	h.Write([]byte(c.ContextName))             //nolint:errcheck
	h.Write([]byte(c.Loader))                  //nolint:errcheck
	h.Write([]byte(c.Namespace))               //nolint:errcheck

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
func (authentication *Authentication) BuildSNMPParams(deviceIP string, port uint16) (*gosnmp.GoSNMP, error) {
	if authentication.Community == "" && authentication.User == "" {
		return nil, errors.New("No authentication mechanism specified")
	}

	var version gosnmp.SnmpVersion
	if authentication.Version == "1" {
		version = gosnmp.Version1
	} else if authentication.Version == "2" || (authentication.Version == "" && authentication.Community != "") {
		version = gosnmp.Version2c
	} else if authentication.Version == "3" || (authentication.Version == "" && authentication.User != "") {
		version = gosnmp.Version3
	} else {
		return nil, fmt.Errorf("SNMP version not supported: %s", authentication.Version)
	}

	authProtocol, err := gosnmplib.GetAuthProtocol(authentication.AuthProtocol)
	if err != nil {
		return nil, err
	}

	privProtocol, err := gosnmplib.GetPrivProtocol(authentication.PrivProtocol)
	if err != nil {
		return nil, err
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if authentication.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if authentication.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Target:          deviceIP,
		Port:            port,
		Community:       authentication.Community,
		Transport:       "udp",
		Version:         version,
		Timeout:         time.Duration(authentication.Timeout) * time.Second,
		Retries:         authentication.Retries,
		SecurityModel:   gosnmp.UserSecurityModel,
		MsgFlags:        msgFlags,
		ContextEngineID: authentication.ContextEngineID,
		ContextName:     authentication.ContextName,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 authentication.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: authentication.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        authentication.PrivKey,
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
