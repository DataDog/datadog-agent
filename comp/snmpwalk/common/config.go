// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common TODO
package common

import (
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
)

// SnmpwalkConfig contains configuration for Snmpwalk collector.
type SnmpwalkConfig struct {
	Enabled     bool `mapstructure:"enabled"`
	StopTimeout int  `mapstructure:"stop_timeout"`
}

// SetDefaults sets default values wherever possible, returning an error if
// any values are malformed.
func (mainConfig *SnmpwalkConfig) SetDefaults(namespace string, logger log.Component) error {
	if mainConfig.StopTimeout == 0 {
		mainConfig.StopTimeout = common.DefaultStopTimeout
	}

	// TODO: FIX UNUSED
	_ = namespace
	_ = logger

	return nil
}
