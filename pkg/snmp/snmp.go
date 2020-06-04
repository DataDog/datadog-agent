// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

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

	"github.com/soniah/gosnmp"
	"github.com/spf13/viper"
)

const (
	defaultPort    = 161
	defaultTimeout = 5
	defaultRetries = 3
)

// ListenerConfig holds global configuration for SNMP discovery
type ListenerConfig struct {
	Workers           int      `mapstructure:"workers"`
	DiscoveryInterval int      `mapstructure:"discovery_interval"`
	AllowedFailures   int      `mapstructure:"allowed_failures"`
	Configs           []Config `mapstructure:"configs"`
}

// Config holds configuration for a particular subnet
type Config struct {
	Network            string          `mapstructure:"network"`
	Port               uint16          `mapstructure:"port"`
	Version            string          `mapstructure:"version"`
	Timeout            int             `mapstructure:"timeout"`
	Retries            int             `mapstructure:"retries"`
	Community          string          `mapstructure:"community"`
	User               string          `mapstructure:"user"`
	AuthKey            string          `mapstructure:"authentication_key"`
	AuthProtocol       string          `mapstructure:"authentication_protocol"`
	PrivKey            string          `mapstructure:"privacy_key"`
	PrivProtocol       string          `mapstructure:"privacy_protocol"`
	ContextEngineID    string          `mapstructure:"context_engine_id"`
	ContextName        string          `mapstructure:"context_name"`
	IgnoredIPAddresses map[string]bool `mapstructure:"ignored_ip_addresses"`
	ADIdentifier       string          `mapstructure:"ad_identifier"`
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
func (c *Config) BuildSNMPParams() (*gosnmp.GoSNMP, error) {
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
