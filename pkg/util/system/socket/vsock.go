// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !clusterchecks

package socket

import (
	"fmt"

	"github.com/mdlayher/vsock"
)

// ParseVSockAddress parses a vsock address and returns the CID
func ParseVSockAddress(addr string) (uint32, error) {
	switch addr {
	case "host":
		return vsock.Host, nil
	case "hypervisor":
		return vsock.Hypervisor, nil
	case "local":
		return vsock.Local, nil
	}
	return 0, fmt.Errorf("invalid vsock address '%s'", addr)
}
