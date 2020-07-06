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
	Version          uint16   `mapstructure:"version" yaml:"version"`
	Port             uint16   `mapstructure:"port" yaml:"port"`
	CommunityStrings []string `mapstructure:"community_strings" yaml:"community_strings"`
	BindHost         string   `mapstructure:"bind_host" yaml:"bind_host"`
}

// trapLogger is a GoSNMP logger interface implementation.
type trapLogger struct {
	gosnmp.Logger
}

func (x *trapLogger) Print(v ...interface{}) {
	// GoSNMP logs show the exact content of decoded trap packets. Logging as DEBUG would be too noisy.
	log.Trace(v...)
}

func (x *trapLogger) Printf(format string, v ...interface{}) {
	log.Tracef(format, v...)
}

// BuildParams returns a valid GoSNMP params structure from a listener configuration.
func (c *TrapListenerConfig) BuildParams() (*gosnmp.GoSNMP, error) {
	if c.Version != 0 && c.Version != 2 {
		return nil, fmt.Errorf("Only `version: 2` is supported for now, got %d", c.Version)
	}

	if c.Port == 0 {
		return nil, errors.New("`port` is required")
	}

	if c.CommunityStrings == nil || len(c.CommunityStrings) == 0 {
		return nil, errors.New("`community_strings` is required")
	}

	params := &gosnmp.GoSNMP{
		Port:      c.Port,
		Transport: "udp",
		Version:   gosnmp.Version2c,
		Logger:    &trapLogger{},
	}

	return params, nil
}
