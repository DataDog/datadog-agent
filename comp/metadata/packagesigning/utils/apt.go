// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"bufio"
	"os"
)

const (
	packageConfig = "/etc/dpkg/dpkg.cfg"
)

// IsPackageSigningEnabled returns the signature policy for the host. When no-debsig is written (and uncommented) in the configuration it means GPG package signing verification is disabled
func IsPackageSigningEnabled() (bool, error) {
	if _, err := os.Stat(packageConfig); err != nil {
		return false, err
	}
	if file, err := os.Open(packageConfig); err == nil {
		defer file.Close()
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			if scanner.Text() == "no-debsig" {
				return false, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, err
		}
	} else {
		return false, err
	}
	return true, nil
}
