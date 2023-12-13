// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import (
	e2eClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// YumPackageManager struct for the Yum package manager
type YumPackageManager struct {
	vmClient e2eClient.VM
}

// NewYumPackageManager return yum package manager
func NewYumPackageManager(vmClient e2eClient.VM) *YumPackageManager {
	return &YumPackageManager{vmClient: vmClient}
}

// Remove executes remove command from yum
func (s *YumPackageManager) Remove(pkg string) (string, error) {
	return s.vmClient.ExecuteWithError("sudo yum remove -y " + pkg)
}
