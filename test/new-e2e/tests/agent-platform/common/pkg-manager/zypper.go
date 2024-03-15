// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Zypper is a package manager for zypper
type Zypper struct {
	host *components.RemoteHost
}

var _ PackageManager = &Zypper{}

// NewZypper return zypper package manager
func NewZypper(host *components.RemoteHost) *Zypper {
	return &Zypper{host: host}
}

// Remove executes remove command from zypper
func (s *Zypper) Remove(pkg string) (string, error) {
	return s.host.Execute("sudo zypper remove -y " + pkg)
}
