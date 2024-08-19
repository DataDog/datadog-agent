// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package setup

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// overrideRunInCoreAgentConfig is a no-op on Linux.
func overrideRunInCoreAgentConfig(_ pkgconfigmodel.Config) {
}
