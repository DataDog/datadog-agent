// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pkgmanager

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
)

// Apt struct for Apt package manager
type Apt struct {
	host *components.RemoteHost
}

var _ PackageManager = &Apt{}

// NewApt return apt package manager
func NewApt(host *components.RemoteHost) *Apt {
	return &Apt{host: host}
}

// Remove call remove from apt
func (s *Apt) Remove(pkg string) (string, error) {
	return s.host.Execute(fmt.Sprintf("sudo apt remove -q -y %s", pkg))
}
