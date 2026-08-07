// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows || aix

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// CheckName is the name of the check
	CheckName = "network"
)

// NetworkCheck represents a network check. networkStats and networkInstanceConfig are
// declared per-platform (network_linux.go, network_windows.go, network_aix.go).
type NetworkCheck struct {
	core.CheckBase
	net    networkStats
	config networkConfig
}

type networkInitConfig struct{}

type networkConfig struct {
	instance networkInstanceConfig
	initConf networkInitConfig
}

// Factory creates a new check factory
func Factory(cfg config.Component) option.Option[func() check.Check] {
	return option.New(func() check.Check {
		return newCheck(cfg)
	})
}
