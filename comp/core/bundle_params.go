// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package core

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
)

// BundleParams defines the parameters for this bundle.
//
// Logs-related parameters are implemented as unexported fields containing
// callbacks.  These fields can be set with the `LogXxx()` methods, which
// return the updated BundleParams.  One of `log.ForOneShot` or `log.ForDaemon`
// must be called.
type BundleParams struct {
	ConfigParams
	SysprobeConfigParams
	LogParams
}

// ConfigParams defines the parameters of the config component
type ConfigParams = config.Params

// LogParams defines the parameters of the log component
type LogParams = log.Params

// SysprobeConfigParams defines the parameters of the system-probe config component
type SysprobeConfigParams = sysprobeconfigimpl.Params
