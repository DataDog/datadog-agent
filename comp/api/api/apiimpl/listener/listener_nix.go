// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package listener

import (
	"net"
)

// platformSpecificListener returns a unix net.Listener for linux platforms
func platformSpecificListener(address string) (net.Listener, error) {
	return net.Listen("unix", address)
}

func hasPlatformSupport() bool {
	return true
}
