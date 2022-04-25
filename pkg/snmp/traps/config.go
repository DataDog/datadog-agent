// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/gosnmp/gosnmp"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled() bool {
	return config.Datadog.GetBool("snmp_traps_enabled")
}

// UserV3 contains the definition of one SNMPv3 user with its username and its auth
// parameters.
type UserV3 struct {
	Username     string `mapstructure:"user" yaml:"user"`
	AuthKey      string `mapstructure:"authKey" yaml:"authKey"`
	AuthProtocol string `mapstructure:"authProtocol" yaml:"authProtocol"`
	PrivKey      string `mapstructure:"privKey" yaml:"privKey"`
	PrivProtocol string `mapstructure:"privProtocol" yaml:"privProtocol"`
}

// Config contains configuration for SNMP trap listeners.
// YAML field tags provided for test marshalling purposes.
type Config struct {
	Port                  uint16   `mapstructure:"port" yaml:"port"`
	Users                 []UserV3 `mapstructure:"users" yaml:"users"`
	CommunityStrings      []string `mapstructure:"community_strings" yaml:"community_strings"`
	BindHost              string   `mapstructure:"bind_host" yaml:"bind_host"`
	StopTimeout           int      `mapstructure:"stop_timeout" yaml:"stop_timeout"`
	Namespace             string   `mapstructure:"namespace" yaml:"namespace"`
	authoritativeEngineID string   `mapstructure:"-" yaml:"-"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(agentHostname string) (*Config, error) {
	var c Config
	err := config.Datadog.UnmarshalKey("snmp_traps_config", &c)
	if err != nil {
		return nil, err
	}

	// gosnmp only supports one v3 user at the moment.
	if len(c.Users) > 1 {
		return nil, errors.New("only one user is currently supported in snmp_traps_config")
	}

	// Set defaults.
	if c.Port == 0 {
		c.Port = defaultPort
	}
	if c.BindHost == "" {
		// Default to global bind_host option.
		c.BindHost = "0.0.0.0"
	}
	if c.StopTimeout == 0 {
		c.StopTimeout = defaultStopTimeout
	}

	if agentHostname == "" {
		// Make sure to have at least some unique bytes for the authoritative engineID.
		// Unlikely to happen since the agent cannot start without a hostname
		agentHostname = "unknown-datadog-agent"
	}
	h := fnv.New128()
	h.Write([]byte(agentHostname))
	// First byte is always 0x80
	// Next four bytes are the Private Enterprise Number (set to an invalid value here)
	// The next 16 bytes are the hash of the agent hostname
	engineID := h.Sum([]byte{0x80, 0xff, 0xff, 0xff, 0xff})
	c.authoritativeEngineID = string(engineID)

	if c.Namespace == "" {
		c.Namespace = config.Datadog.GetString("network_devices.namespace")
	}
	c.Namespace, err = common.NormalizeNamespace(c.Namespace)
	if err != nil {
		return nil, fmt.Errorf("invalid snmp_traps_config: %w", err)
	}

	return &c, nil
}

// Addr returns the host:port address to listen on.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.BindHost, c.Port)
}

// BuildSNMPParams returns a valid GoSNMP params structure from configuration.
func (c *Config) BuildSNMPParams() (*gosnmp.GoSNMP, error) {
	if len(c.Users) == 0 {
		return &gosnmp.GoSNMP{
			Port:      c.Port,
			Transport: "udp",
			Version:   gosnmp.Version2c, // No user configured, let's use Version2 which is enough and doesn't require setting up fake security data.
			Logger:    gosnmp.NewLogger(&trapLogger{}),
		}, nil
	}
	user := c.Users[0]
	var authProtocol gosnmp.SnmpV3AuthProtocol
	switch lowerAuthProtocol := strings.ToLower(user.AuthProtocol); lowerAuthProtocol {
	case "":
		authProtocol = gosnmp.NoAuth
	case "md5":
		authProtocol = gosnmp.MD5
	case "sha":
		authProtocol = gosnmp.SHA
	default:
		return nil, fmt.Errorf("unsupported authentication protocol: %s", user.AuthProtocol)
	}

	var privProtocol gosnmp.SnmpV3PrivProtocol
	switch lowerPrivProtocol := strings.ToLower(user.PrivProtocol); lowerPrivProtocol {
	case "":
		privProtocol = gosnmp.NoPriv
	case "des":
		privProtocol = gosnmp.DES
	case "aes":
		privProtocol = gosnmp.AES
	case "aes192":
		privProtocol = gosnmp.AES192
	case "aes192c":
		privProtocol = gosnmp.AES192C
	case "aes256":
		privProtocol = gosnmp.AES256
	case "aes256c":
		privProtocol = gosnmp.AES256C
	default:
		return nil, fmt.Errorf("unsupported privacy protocol: %s", user.PrivProtocol)
	}

	msgFlags := gosnmp.NoAuthNoPriv
	if user.PrivKey != "" {
		msgFlags = gosnmp.AuthPriv
	} else if user.AuthKey != "" {
		msgFlags = gosnmp.AuthNoPriv
	}

	return &gosnmp.GoSNMP{
		Port:          c.Port,
		Transport:     "udp",
		Version:       gosnmp.Version3, // Always using version3 for traps, only option that works with all SNMP versions simultaneously
		SecurityModel: gosnmp.UserSecurityModel,
		MsgFlags:      msgFlags,
		SecurityParameters: &gosnmp.UsmSecurityParameters{
			UserName:                 user.Username,
			AuthoritativeEngineID:    c.authoritativeEngineID,
			AuthenticationProtocol:   authProtocol,
			AuthenticationPassphrase: user.AuthKey,
			PrivacyProtocol:          privProtocol,
			PrivacyPassphrase:        user.PrivKey,
		},
		Logger: gosnmp.NewLogger(&trapLogger{}),
	}, nil
}
