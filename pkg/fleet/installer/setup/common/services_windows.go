// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package common

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// restartServices restarts the services that need to be restarted after a package upgrade or
// an install script re-run; because the configuration may have changed.
func (s *Setup) restartServices(pkgs []packageWithVersion) error {
	for _, pkg := range pkgs {
		switch pkg.name {
		case DatadogAgentPackage:
			if err := winutil.RestartService("datadogagent"); err != nil {
				return err
			}
		}
	}
	return nil
}
