// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"path/filepath"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const (
	privateActionRunnerRelativeDir = "private-action-runner"

	createConnectionEndpoint = "/api/v2/actions/connections"
	apiKeyHeader             = "DD-API-KEY"
	appKeyHeader             = "DD-APPLICATION-KEY"
	contentTypeHeader        = "Content-Type"
	contentType              = "application/vnd.api+json"
	userAgentHeader          = "User-Agent"
)

func getPrivateActionRunnerDir() string {
	return filepath.Join(defaultpaths.ConfPath, privateActionRunnerRelativeDir)
}
func getScriptConfigPath() string {
	filename := "script-config.yaml"
	if runtime.GOOS == "windows" {
		filename = "powershell-script-config.yaml"
	}
	return filepath.Join(getPrivateActionRunnerDir(), filename)
}
