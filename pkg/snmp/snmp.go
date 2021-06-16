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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/viper"
	"github.com/gosnmp/gosnmp"
)

const (
	defaultPort    = 161
	defaultTimeout = 5
	defaultRetries = 3
)

// ListenerConfig holds global configuration for SNMP discovery
type ListenerConfig struct {
	Workers               int      `mapstructure:"workers"`
	DiscoveryInterval     int      `mapstructure:"discovery_interval"`
	AllowedFailures       int      `mapstructure:"discovery_allowed_failures"`
	Loader                string   `mapstructure:"loader"`
	CollectDeviceMetadata bool     `mapstructure:"collect_device_metadata"`
	Configs               []Config `mapstructure:"configs"`

	// legacy
	AllowedFailuresLegacy int `mapstructure:"allowed_failures"`
}

// Config holds configuration for a particular subnet
type Config struct {
	Network                     string          `mapstructure:"network_address"`
	Port                        uint16          `mapstructure:"port"`
	Version                     string          `mapstructure:"snmp_version"`
	Timeout                     int             `mapstructure:"timeout"`
	Retries                     int             `mapstructure:"retries"`
	OidBatchSize                int             `mapstructure:"oid_batch_size"`
	Community                   string          `mapstructure:"community_string"`
	User                        string          `mapstructure:"user"`
	AuthKey                     string          `mapstructure:"authKey"`
	AuthProtocol                string          `mapstructure:"authProtocol"`
	PrivKey                     string          `mapstructure:"privKey"`
	PrivProtocol                string          `mapstructure:"privProtocol"`
	ContextEngineID             string          `mapstructure:"context_engine_id"`
	ContextName                 string          `mapstructure:"context_name"`
	IgnoredIPAddresses          map[string]bool `mapstructure:"ignored_ip_addresses"`
	ADIdentifier                string          `mapstructure:"ad_identifier"`
	Loader                      string          `mapstructure:"loader"`
	CollectDeviceMetadataConfig *bool           `mapstructure:"collect_device_metadata"`
	CollectDeviceMetadata       bool
	Tags                        []string `mapstructure:"tags"`

	// Legacy
	NetworkLegacy      string `mapstructure:"network"`
	VersionLegacy      string `mapstructure:"version"`
	CommunityLegacy    string `mapstructure:"community"`
	AuthKeyLegacy      string `mapstructure:"authentication_key"`
	AuthProtocolLegacy string `mapstructure:"authentication_protocol"`
	PrivKeyLegacy      string `mapstructure:"privacy_key"`
	PrivProtocolLegacy string `mapstructure:"privacy_protocol"`
}

// NewListenerConfig parses configuration and returns a built ListenerConfig
func NewListenerConfig() (ListenerConfig, error) {
	var snmpConfig ListenerConfig
	opt := viper.DecodeHook(
		func(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
			// Turn an array into a map for ignored addresses
			if rf != reflect.Slice {
				return data, nil
			}
			if rt != reflect.Map {
				return data, nil
			}
			newData := map[interface{}]bool{}
			for _, i := range data.([]interface{}) {
				newData[i] = true
			}
			return newData, nil
		},
	)

	if err := config.Datadog.UnmarshalKey("snmp_listener", &snmpConfig, opt); err != nil {
		return snmpConfig, err
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
		if config.Loader == "" {
			config.Loader = snmpConfig.Loader
		}
		config.Community = firstNonEmpty(config.Community, config.CommunityLegacy)
		config.AuthKey = firstNonEmpty(config.AuthKey, config.AuthKeyLegacy)
		config.AuthProtocol = firstNonEmpty(config.AuthProtocol, config.AuthProtocolLegacy)
		config.PrivKey = firstNonEmpty(config.PrivKey, config.PrivKeyLegacy)
		config.PrivProtocol = firstNonEmpty(config.PrivProtocol, config.PrivProtocolLegacy)
		config.Network = firstNonEmpty(config.Network, config.NetworkLegacy)
		config.Version = firstNonEmpty(config.Version, config.VersionLegacy)
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

// BuildSNMPParams returns a valid GoSNMP struct to start making queries
func (c *Config) BuildSNMPParams(deviceIP string) (*gosnmp.GoSNMP, error) {
	if c.Community == "" && c.User == "" {
		return nil, errors.New("No authentication mechanism specified")
	}

	var version gosnmp.SnmpVersion
	if c.Version == "1" {
		version = gosnmp.Version1
	} else if c.Version == "2" || (c.Version == "" && c.Community != "") {
		version = gosnmp.Version2c
	} else if c.Version == "3" || (c.Version == "" && c.User != "") {
		version = gosnmp.Version3
	} else {
		return nil, fmt.Errorf("SNMP version not supported: %s", c.Version)
	}

	var authProtocol gosnmp.SnmpV3AuthProtocol
	lowerAuthProtocol := strings.ToLower(c.AuthProtocol)
	if lowerAuthProtocol == "" {
		authProtocol = gosnmp.NoAuth
	} else if lowerAuthProtocol == "md5" {
		authProtocol = gosnmp.MD5
	} else if lowerAuthProtocol == "sha" {
		authProtocol = gosnmp.SHA
	} else {
		return nil, fmt.Errorf("Unsupported authentication protocol: %s", c.AuthProtocol)
	}

	var privProtocol gosnmp.SnmpV3PrivProtocol
	lowerPrivProtocol := strings.ToLower(c.PrivProtocol)
	if lowerPrivProtocol == "" {
		privProtocol = gosnmp.NoPriv
	} else if lowerPrivProtocol == "des" {
		privProtocol = gosnmp.DES
	} else if lowerPrivProtocol == "aes" {
		privProtocol = gosnmp.AES
	} else if lowerPrivProtocol == "aes192" {
		privProtocol = gosnmp.AES192
	} else if lowerPrivProtocol == "aes192c" {
		privProtocol = gosnmp.AES192C
	} else if lowerPrivProtocol == "aes256" {
		privProtocol = gosnmp.AES256
	} else if lowerPrivProtocol == "aes256c" {
		privProtocol = gosnmp.AES256C
	} else {
		return nil, fmt.Errorf("Unsupported privacy protocol: %s", c.PrivProtocol)
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if c.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if c.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Target:          deviceIP,
		Port:            c.Port,
		Community:       c.Community,
		Transport:       "udp",
		Version:         version,
		Timeout:         time.Duration(c.Timeout) * time.Second,
		Retries:         c.Retries,
		SecurityModel:   gosnmp.UserSecurityModel,
		MsgFlags:        msgFlags,
		ContextEngineID: c.ContextEngineID,
		ContextName:     c.ContextName,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 c.User,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: c.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        c.PrivKey,
		},
	}, nil
}

// IsIPIgnored checks the given IP against IgnoredIPAddresses
func (c *Config) IsIPIgnored(ip net.IP) bool {
	ipString := ip.String()
	_, present := c.IgnoredIPAddresses[ipString]
	return present
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
