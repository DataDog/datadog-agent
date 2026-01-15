// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package socket

import "strings"

// GetFamilyAddress retur1-ns the address famility to use for a given address
func GetFamilyAddress(path string) string {
	if strings.HasPrefix(path, "/") {
		return "unix"
	}
	return "tcp"
}

// GetSocketAddress returns the address famility to use for a given address
func GetSocketAddress(path string) (string, string) {
	if strings.HasPrefix(path, "/") {
		return "unix", path
	} else if after, ok := strings.CutPrefix(path, "vsock:"); ok {
		return "vsock", after
	}
	return "tcp", path
}
