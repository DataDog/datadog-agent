// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils defines shared methods in package signing component
package utils

import (
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/comp/core/log"
)

// GetLinuxGlobalSigningPolicies returns:
// * if package signing is enabled on the host
// * if repository signing is enabled on the
func GetLinuxGlobalSigningPolicies(logger log.Component) (bool, bool) {
	if runtime.GOOS == "linux" {
		pkgManager := GetPackageManager()
		switch pkgManager {
		case "apt":
			pkgSigning, err := IsPackageSigningEnabled()
			if err != nil {
				logger.Debugf("Error while reading main config file: %s", err)
			}
			return pkgSigning, false
		case "yum", "dnf", "zypper":
			pkgSigning, repoSigning, err := getMainGPGCheck(pkgManager)
			if err != nil {
				logger.Debugf("Error while reading main config file: %s", err)
			}
			return pkgSigning, repoSigning
		default: // should not happen, tested above
			return false, false
		}
	}
	return false, false
}

const (
	aptPath  = "/etc/apt"
	yumPath  = "/etc/yum"
	dnfPath  = "/etc/dnf"
	zyppPath = "/etc/zypp"
)

// GetPackageManager is a lazy implementation to detect if we use APT or YUM (RH or SUSE)
func GetPackageManager() string {
	if _, err := os.Stat(aptPath); err == nil {
		return "apt"
	} else if _, err := os.Stat(yumPath); err == nil {
		return "yum"
	} else if _, err := os.Stat(dnfPath); err == nil {
		return "dnf"
	} else if _, err := os.Stat(zyppPath); err == nil {
		return "zypper"
	}
	return ""
}

// CompareRepoPerKeys is a method used on tests
func CompareRepoPerKeys(a, b map[string][]Repository) []string {
	errorKeys := make([]string, 0)
	if len(a) < len(b) {
		for key := range b {
			if _, ok := a[key]; !ok {
				errorKeys = append(errorKeys, key)
			}
		}
	} else if len(a) > len(b) {
		for key := range a {
			if _, ok := b[key]; !ok {
				errorKeys = append(errorKeys, key)
			}
		}
	} else {
		errorKeys = append(errorKeys, compareKey(a, b)...)
		errorKeys = append(errorKeys, compareKey(b, a)...)
	}
	return errorKeys
}
func compareKey(a, b map[string][]Repository) []string {
	errorKeys := make([]string, 0)
	for key := range a {
		if _, ok := b[key]; !ok {
			errorKeys = append(errorKeys, key)
		} else {
			if len(a[key]) == len(b[key]) {
				if anyMissingRepository(a[key], b[key]) {
					errorKeys = append(errorKeys, key)
				}
				if anyMissingRepository(b[key], a[key]) {
					errorKeys = append(errorKeys, key)
				}
			} else {
				errorKeys = append(errorKeys, key)
			}
		}
	}
	return errorKeys
}
func anyMissingRepository(r, s []Repository) bool {
	for _, src := range r {
		found := false
		for _, dest := range s {
			if src.Name == dest.Name && src.Enabled == dest.Enabled && src.GPGCheck == dest.GPGCheck && src.RepoGPGCheck == dest.RepoGPGCheck {
				found = true
				break
			}

		}
		if !found {
			return true
		}
	}
	return false
}
