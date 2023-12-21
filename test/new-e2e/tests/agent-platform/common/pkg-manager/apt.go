// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pkgmanager contains pkgmanager implementations
package pkgmanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// AptPackageManager struct for Apt package manager
type AptPackageManager struct {
	host *components.RemoteHost
}

// NewAptPackageManager return apt package manager
func NewAptPackageManager(host *components.RemoteHost) *AptPackageManager {
	return &AptPackageManager{host: host}
}

// Remove call remove from apt
func (s *AptPackageManager) Remove(pkg string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo apt remove -q -y %s", pkg))
}
