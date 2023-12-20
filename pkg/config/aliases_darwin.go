// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Aliases to setup package
const (
	DefaultSecurityAgentLogFile = pkgconfigsetup.DefaultSecurityAgentLogFile
	DefaultProcessAgentLogFile  = pkgconfigsetup.DefaultProcessAgentLogFile
	DefaultDDAgentBin           = pkgconfigsetup.DefaultDDAgentBin
)
