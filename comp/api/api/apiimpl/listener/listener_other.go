// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

package listener

import (
	"net"
)

// platformSpecificListener returns a tcp net.Listener by default for non-Linux
func platformSpecificListener(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}

// hasPlatformSupport returns whether there is socket support for the operating system
func hasPlatformSupport() bool {
	return false
}
