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

	"golang.org/x/sys/windows/registry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (
	// defaultSystemProbeAddress is the default address to be used for connecting to the system probe
	defaultSystemProbeAddress = "localhost:3333"

	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-system-probe"

	agentRegistryKey       = `SOFTWARE\DataDog\Datadog Agent`
	closedSourceKeyName    = "AllowClosedSource"
	closedSourceAllowed    = 1
	closedSourceNotAllowed = 0
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
	regKey, err := registry.OpenKey(registry.LOCAL_MACHINE, agentRegistryKey, registry.QUERY_VALUE)
	if err != nil {
		log.Warnf("unable to open registry key %s: %v", agentRegistryKey, err)
		return false
	}
	defer regKey.Close()

	val, _, err := regKey.GetIntegerValue(closedSourceKeyName)
	if err != nil {
		log.Warnf("unable to get value for %s: %v", closedSourceKeyName, err)
		return false
	}

	if val == closedSourceAllowed {
		log.Debug("closed-source software allowed")
		return true
	} else if val == closedSourceNotAllowed {
		log.Debug("closed-source software not allowed")
		return false
	} else {
		log.Debugf("unexpected value set for %s: %d", closedSourceKeyName, val)
		return false
	}
}
