// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package installer contains tests for the datadog installer
package installer

import (
	"fmt"
	"os"
)

// InstallMethodOption is the type for the install method to use for the tests
type InstallMethodOption string

const (
	// InstallMethodInstallScript is the default install method
	InstallMethodInstallScript InstallMethodOption = "install_script"
	// InstallMethodAnsible is the install method for Ansible
	InstallMethodAnsible InstallMethodOption = "ansible"
	// InstallMethodWindows is the install method for Windows
	InstallMethodWindows InstallMethodOption = "windows"
)

// GetInstallMethodFromEnv returns the install method to use for the tests
func GetInstallMethodFromEnv() InstallMethodOption {
	supportedValues := []string{string(InstallMethodAnsible), string(InstallMethodInstallScript), string(InstallMethodWindows)}
	envValue := os.Getenv("FLEET_INSTALL_METHOD")
	switch envValue {
	case "install_script":
		return InstallMethodInstallScript
	case "ansible":
		return InstallMethodAnsible
	case "windows":
		return InstallMethodWindows
	default:
		panic(fmt.Sprintf("unsupported install method: %s. Supported values are: %v", envValue, supportedValues))
	}
}
