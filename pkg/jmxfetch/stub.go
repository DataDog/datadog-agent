// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !jmx

package jmxfetch

import (
	"github.com/DataDog/datadog-agent/comp/agent/jmxlogger"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
)

// InitRunner is a stub for builds that do not include jmx
func InitRunner(_ dogstatsdServer.Component, _ jmxlogger.Component, _ ipc.Component) {}

// RegisterWith adds the JMX scheduler to receive events from the autodiscovery.
// Noop version for builds without jmx.
func RegisterWith(_ autodiscovery.Component) {}

// StopJmxfetch does nothing when the agent does not ship jmx
func StopJmxfetch() {}

// GetIntegrations returns an empty result when the agent does not ship jmx
func GetIntegrations() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}
