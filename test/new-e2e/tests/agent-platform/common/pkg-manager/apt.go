// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pkgmanager contains pkgmanager implementations
package pkgmanager

import (
	"fmt"

	e2eClient "github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
)

// AptPackageManager struct for Apt package manager
type AptPackageManager struct {
	vmClient e2eClient.VM
}

// NewAptPackageManager return apt package manager
func NewAptPackageManager(vmClient e2eClient.VM) *AptPackageManager {
	return &AptPackageManager{vmClient: vmClient}
}

// Remove call remove from apt
func (s *AptPackageManager) Remove(pkg string) (string, error) {
	return s.vmClient.ExecuteWithError(fmt.Sprintf("sudo apt remove -q -y %s", pkg))
}
