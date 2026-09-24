// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package msi contains helper functions to work with msi packages.
//
// The package provides automatic retry functionality for MSI operations using exponential backoff
// to handle transient errors, particularly exit code 1618 (ERROR_INSTALL_ALREADY_RUNNING)
// which occurs when another MSI installation is in progress.
package msi

import (
	"errors"
	"fmt"
	"path/filepath"
)

// AgentMSIName returns the MSI filename for an Agent package version.
func AgentMSIName(version string, fipsMode bool) string {
	prefix := "datadog-agent"
	if fipsMode {
		prefix = "datadog-fips-agent"
	}
	return fmt.Sprintf("%s-%s-x86_64.msi", prefix, version)
}

// AgentProductName returns the Agent's Windows Installer product name.
func AgentProductName(fipsMode bool) string {
	if fipsMode {
		return "Datadog FIPS Agent"
	}
	return "Datadog Agent"
}

// FindAgentMSI finds exactly one Agent MSI of the requested flavor.
func FindAgentMSI(dir string, fipsMode bool) (string, error) {
	msis, err := filepath.Glob(filepath.Join(dir, AgentMSIName("*", fipsMode)))
	if err != nil {
		return "", err
	}
	if len(msis) > 1 {
		return "", errors.New("too many MSIs in package")
	}
	if len(msis) == 0 {
		return "", errors.New("no MSIs in package")
	}
	return msis[0], nil
}
