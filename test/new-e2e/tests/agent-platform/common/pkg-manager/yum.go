// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Yum struct for the Yum package manager
type Yum struct {
	host *components.RemoteHost
}

var _ PackageManager = &Yum{}

// NewYum return yum package manager
func NewYum(host *components.RemoteHost) *Yum {
	return &Yum{host: host}
}

// Remove executes remove command from yum
func (s *Yum) Remove(pkg string) (string, error) {
	return s.host.Execute("sudo yum remove -y " + pkg)
}
