// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package config

import (
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
	"golang.org/x/sys/windows/registry"
)

const (
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"

	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-system-probe"

	AgentRegistryKey       = `SOFTWARE\DataDog\Datadog Agent`
	ClosedSourceKeyName    = "AllowClosedSource"
	ClosedSourceAllowed    = 1
	ClosedSourceNotAllowed = 0
)

var (
	defaultConfigDir = "c:\\programdata\\datadog\\"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		defaultConfigDir = pd
	}
}

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockAddress string) error {
	if _, _, err := net.SplitHostPort(sockAddress); err != nil {
		return fmt.Errorf("socket address must be of the form 'host:port'")
	}
	return nil
}

func isClosedSourceAllowed() bool {
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, AgentRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", AgentRegistryKey, err)
		return false
	}
	defer regKey.Close()

	val, _, err := regKey.GetIntegerValue(ClosedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", ClosedSourceKeyName, err)
		return false
	}

	if val == ClosedSourceAllowed {
		log.Info("closed-source software allowed")
		return true
	} else if val == ClosedSourceNotAllowed {
		log.Info("closed-source software not allowed")
		return false
	} else {
		log.Infof("unexpected value set for %s: %d", ClosedSourceKeyName, val)
		return false
	}
}
