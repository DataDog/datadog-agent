// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pkgmanager contains pkgmanager implementations
package pkgmanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
)

// AptPackageManager struct for Apt package manager
type AptPackageManager struct {
	env *e2e.AgentEnv
}

// NewAptPackageManager return apt package manager
func NewAptPackageManager(env *e2e.AgentEnv) *AptPackageManager {
	return &AptPackageManager{env}
}

// Remove call remove from apt
func (s *AptPackageManager) Remove(pkg string) (string, error) {
	return s.env.VM.ExecuteWithError(fmt.Sprintf("sudo apt-get remove -q -y %s", pkg))
}
