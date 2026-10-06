// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"path/filepath"
	"runtime"

	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
)

const (
	createConnectionEndpoint = "/api/v2/actions/connections"
	apiKeyHeader             = "DD-API-KEY"
	appKeyHeader             = "DD-APPLICATION-KEY"
	contentTypeHeader        = "Content-Type"
	contentType              = "application/vnd.api+json"
	userAgentHeader          = "User-Agent"
)

func getPrivateActionRunnerDir() string {
	return parconfig.DefaultScriptCredentialFileRoot()
}
func getScriptConfigPath() string {
	filename := "script-config.yaml"
	if runtime.GOOS == "windows" {
		filename = "powershell-script-config.yaml"
	}
	return filepath.Join(getPrivateActionRunnerDir(), filename)
}
