// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package agent includes helpers related to the Datadog Agent on Windows
package agent

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows"
)

// GetDatadogAgentProductCode returns the product code GUID for the Datadog Agent
func GetDatadogAgentProductCode(client client.VM) (string, error) {
	return windows.GetProductCodeByName(client, "Datadog Agent")
}

// UninstallAgent uninstalls the Datadog Agent
func UninstallAgent(client client.VM, logPath string) error {
	product, err := GetDatadogAgentProductCode(client)
	if err != nil {
		return err
	}
	return windows.UninstallMSI(client, product, logPath)
}
