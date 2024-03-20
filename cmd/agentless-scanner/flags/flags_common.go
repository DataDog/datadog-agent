// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package flags holds flags related files
package flags

import (
	"github.com/DataDog/datadog-agent/pkg/agentless/types"
)

// GlobalFlags contains the global flags
var GlobalFlags struct {
	ConfigFilePath string
	DiskMode       types.DiskMode
	DefaultActions []types.ScanAction
	NoForkScanners bool
}
