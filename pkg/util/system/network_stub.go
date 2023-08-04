// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package system

import "net"

// ParseProcessRoutes is just a stub for platforms where that's currently not
// defined (like MacOS). This allows code that refers to this (like the docker
// check) to at least compile in those platforms, and that's useful for things
// like running unit tests.
func ParseProcessRoutes(procPath string, pid int) ([]NetworkRoute, error) {
	panic("ParseProcessRoutes is not implemented in this environment")
}

// GetDefaultGateway is just a stub for platforms where that's currently not
// defined (like MacOS). This allows code that refers to this (like the cluster
// agent) to at least compile in those platforms, and that's useful for things
// like running unit tests.
func GetDefaultGateway(procPath string) (net.IP, error) {
	panic("GetDefaultGateway is not implemented in this environment")
}
