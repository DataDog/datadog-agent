// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// YumPackageManager struct for the Yum package manager
type YumPackageManager struct {
	host *components.RemoteHost
}

// NewYumPackageManager return yum package manager
func NewYumPackageManager(host *components.RemoteHost) *YumPackageManager {
	return &YumPackageManager{host: host}
}

// Remove executes remove command from yum
func (s *YumPackageManager) Remove(pkg string) (string, error) {
	return s.host.Execute("sudo yum remove -y " + pkg)
}
