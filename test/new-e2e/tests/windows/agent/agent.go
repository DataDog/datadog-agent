// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent includes helpers related to the Datadog Agent on Windows
package agent

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
)

// GetDatadogAgentProductCode returns the product code GUID for the Datadog Agent
func GetDatadogAgentProductCode(host *components.RemoteHost) (string, error) {
	return windows.GetProductCodeByName(host, "Datadog Agent")
}

// UninstallAgent uninstalls the Datadog Agent
func UninstallAgent(host *components.RemoteHost, logPath string) error {
	product, err := GetDatadogAgentProductCode(host)
	if err != nil {
		return err
	}
	return windows.UninstallMSI(host, product, logPath)
}
