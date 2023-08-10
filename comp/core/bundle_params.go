// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
)

// BundleParams defines the parameters for this bundle.
//
// Logs-related parameters are implemented as unexported fields containing
// callbacks.  These fields can be set with the `LogXxx()` methods, which
// return the updated BundleParams.  One of `ForOneShot` or `ForDaemon`
// must be called.
type BundleParams struct {
	ConfigParams
	SysprobeConfigParams
	LogParams
}

// ConfigParams exported type should have comment or be unexported
type ConfigParams = config.Params
// LogParams exported type should have comment or be unexported
type LogParams = log.Params
// SysprobeConfigParams exported type should have comment or be unexported
type SysprobeConfigParams = sysprobeconfig.Params
