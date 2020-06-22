// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package traps

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/soniah/gosnmp"
)

// TrapListenerConfig contains configuration for an SNMP trap listener.
// YAML field tags provided for test marshalling purposes.
type TrapListenerConfig struct {
	Port      uint16   `mapstructure:"port" yaml:"port"`
	Version   string   `mapstructure:"version" yaml:"version"`
	Community []string `mapstructure:"community" yaml:"community"`
}

// trapLogger is a GoSNMP logger interface implementation.
type trapLogger struct {
	gosnmp.Logger
}

func (x *trapLogger) Print(v ...interface{}) {
	log.Debug(v...)
}

func (x *trapLogger) Printf(format string, v ...interface{}) {
	log.Debugf(format, v...)
}

// BuildVersion returns a GoSNMP version value from a string value, rejecting unsupported SNMP versions.
func BuildVersion(value string) (gosnmp.SnmpVersion, error) {
	switch value {
	case "2", "2c":
		return gosnmp.Version2c, nil
	default:
		return 0, fmt.Errorf("Unsupported version: '%s' (possible values are '2' (or the '2c' alias))", value)
	}
}

// BuildParams returns a valid GoSNMP params structure from a listener configuration.
func (c *TrapListenerConfig) BuildParams() (*gosnmp.GoSNMP, error) {
	if c.Port == 0 {
		return nil, errors.New("`port` is required")
	}

	if c.Version == "" {
		c.Version = "2"
	}
	version, err := BuildVersion(c.Version)
	if err != nil {
		return nil, err
	}

	requiresCommunity := version == gosnmp.Version2c
	communitySet := c.Community != nil && len(c.Community) > 0
	if requiresCommunity && !communitySet {
		return nil, errors.New("`community` is required")
	}

	params := &gosnmp.GoSNMP{
		Port:      c.Port,
		Transport: "udp",
		Version:   version,
		Logger:    &trapLogger{},
	}

	return params, nil
}
