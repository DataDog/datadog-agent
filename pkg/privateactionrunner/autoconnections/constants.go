// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/defaultpaths"
)

const (
	PrivateActionRunnerRelativeDir = "private-action-runner"
	ScriptConfigFileName           = "script-config.yaml"

	CreateConnectionEndpoint = "/api/v2/actions/connections"
	APIKeyHeader             = "DD-API-KEY"
	AppKeyHeader             = "DD-APPLICATION-KEY"
	ContentTypeHeader        = "Content-Type"
	ContentType              = "application/vnd.api+json"
	UserAgentHeader          = "User-Agent"
)

func GetPrivateActionRunnerDir() string {
	return filepath.Join(defaultpaths.ConfPath, PrivateActionRunnerRelativeDir)
}
func GetScriptConfigPath() string {
	return filepath.Join(GetPrivateActionRunnerDir(), ScriptConfigFileName)
}
