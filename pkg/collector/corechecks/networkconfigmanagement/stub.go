// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ncm

// Package networkconfigmanagement defines the agent core check for retrieving network device configurations (stub implementation)
package networkconfigmanagement

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the network configuration management check.
const CheckName = "network_config_management"

// Factory returns a stub implementation of the network configuration management check.
func Factory(_ config.Component) option.Option[func() check.Check] {
	return option.None[func() check.Check]()
}
