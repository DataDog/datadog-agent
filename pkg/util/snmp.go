// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package util

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/soniah/gosnmp"
)

// SNMPListenerConfig holds global configuration for SNMP discovery
type SNMPListenerConfig struct {
	Workers           int          `mapstructure:"workers"`
	DiscoveryInterval int          `mapstructure:"discovery_interval"`
	Configs           []SNMPConfig `mapstructure:"configs"`
}

// SNMPConfig holds configuration for a particular subnet
type SNMPConfig struct {
	Network         string `mapstructure:"network"`
	Port            uint16 `mapstructure:"port"`
	Version         string `mapstructure:"version"`
	Timeout         int    `mapstructure:"timeout"`
	Retries         int    `mapstructure:"retries"`
	Community       string `mapstructure:"community"`
	User            string `mapstructure:"user"`
	AuthKey         string `mapstructure:"authentication_key"`
	AuthProtocol    string `mapstructure:"authentication_protocol"`
	PrivKey         string `mapstructure:"privacy_key"`
	PrivProtocol    string `mapstructure:"privacy_protocol"`
	ContextEngineID string `mapstructure:"context_engine_id"`
	ContextName     string `mapstructure:"context_name"`
}

// Digest returns an hash value representing the data stored in this configuration, minus the network address
func (c *SNMPConfig) Digest(address string) string {
	h := fnv.New64()
	h.Write([]byte(address))
	h.Write([]byte(fmt.Sprintf("%d", c.Port)))
	h.Write([]byte(c.Version))
	h.Write([]byte(c.Community))
	h.Write([]byte(c.User))
	h.Write([]byte(c.AuthKey))
	h.Write([]byte(c.AuthProtocol))
	h.Write([]byte(c.PrivKey))
	h.Write([]byte(c.PrivProtocol))
	h.Write([]byte(c.ContextEngineID))
	h.Write([]byte(c.ContextName))

	return strconv.FormatUint(h.Sum64(), 16)
}

// BuildSNMPParams returns a valid GoSNMP struct to start making queries
func (c *SNMPConfig) BuildSNMPParams() (*gosnmp.GoSNMP, error) {
	port := c.Port
	if port == 0 {
		port = 161
	}

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
	if c.AuthProtocol == "MD5" {
		authProtocol = gosnmp.MD5
	} else if c.AuthProtocol == "SHA" {
		authProtocol = gosnmp.SHA
	} else if c.AuthProtocol != "" {
		return nil, fmt.Errorf("Unsupported authentication protocol: %s", c.AuthProtocol)
	}

	var privProtocol gosnmp.SnmpV3PrivProtocol
	if c.PrivProtocol == "DES" {
		privProtocol = gosnmp.DES
	} else if c.PrivProtocol == "AES" {
		privProtocol = gosnmp.AES
	} else if c.PrivProtocol != "" {
		return nil, fmt.Errorf("Unsupported privacy protocol: %s", c.PrivProtocol)
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if c.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if c.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Port:            port,
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
