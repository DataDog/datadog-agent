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

// getNoDebsig returns the signature policy for the host. no-debsig means GPG check is enabled
func getNoDebsig() bool {
	if _, err := os.Stat(packageConfig); err == nil {
		if file, err := os.Open(packageConfig); err == nil {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				if scanner.Text() == "no-debsig" {
					return true
				}
			}
		}
	}
	return false
}
