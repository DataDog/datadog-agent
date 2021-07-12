// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows
// +build kubeapiserver

package app

import "github.com/DataDog/datadog-agent/cmd/security-agent/common"

func init() {
	SecurityAgentCmd.AddCommand(common.CheckCmd(confPathArray))
}
