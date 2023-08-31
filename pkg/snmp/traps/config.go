// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

package traps

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/snmp/utils"
)

// IsEnabled returns whether SNMP trap collection is enabled in the Agent configuration.
func IsEnabled(conf config.Component) bool {
	return conf.GetBool("network_devices.snmp_traps.enabled")
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
	Enabled               bool     `mapstructure:"enabled" yaml:"enabled"`
	Port                  uint16   `mapstructure:"port" yaml:"port"`
	Users                 []UserV3 `mapstructure:"users" yaml:"users"`
	CommunityStrings      []string `mapstructure:"community_strings" yaml:"community_strings"`
	BindHost              string   `mapstructure:"bind_host" yaml:"bind_host"`
	StopTimeout           int      `mapstructure:"stop_timeout" yaml:"stop_timeout"`
	Namespace             string   `mapstructure:"namespace" yaml:"namespace"`
	authoritativeEngineID string   `mapstructure:"-" yaml:"-"`
}

// ReadConfig builds and returns configuration from Agent configuration.
func ReadConfig(agentHostname string, conf config.Component) (*Config, error) {
	var c Config
	err := conf.UnmarshalKey("network_devices.snmp_traps", &c)
	if err != nil {
		return nil, err
	}

	if !c.Enabled {
		return nil, errors.New("traps listener is disabled")
	}

	// gosnmp only supports one v3 user at the moment.
	if len(c.Users) > 1 {
		return nil, errors.New("only one user is currently supported in SNMP Traps Listener configuration")
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
		c.Namespace = conf.GetString("network_devices.namespace")
	}
	c.Namespace, err = utils.NormalizeNamespace(c.Namespace)
	if err != nil {
		return nil, fmt.Errorf("unable to load config: %w", err)
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
	authProtocol, err := gosnmplib.GetAuthProtocol(user.AuthProtocol)
	if err != nil {
		return nil, err
	}

	privProtocol, err := gosnmplib.GetPrivProtocol(user.PrivProtocol)
	if err != nil {
		return nil, err
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
