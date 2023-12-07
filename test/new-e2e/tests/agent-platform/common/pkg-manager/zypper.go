// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"

// ZypperPackageManager is a package manager for zypper
type ZypperPackageManager struct {
	vmClient client.VM
}

// NewZypperPackageManager return zypper package manager
func NewZypperPackageManager(vmClient client.VM) *ZypperPackageManager {
	return &ZypperPackageManager{vmClient: vmClient}
}

// Remove executes remove command from zypper
func (s *ZypperPackageManager) Remove(pkg string) (string, error) {
	return s.vmClient.ExecuteWithError("sudo zypper remove -y " + pkg)
}
