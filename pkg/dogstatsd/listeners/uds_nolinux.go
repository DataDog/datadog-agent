// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !linux

package listeners

import (
	"errors"
	"net"
)

// ErrLinuxOnly is emitted on non-linux platforms
var ErrLinuxOnly = errors.New("only implemented on Linux hosts")

// getUDSAncillarySize returns 0 on non-linux hosts
func getUDSAncillarySize() int {
	return 0
}

// enableUDSPassCred returns a "not implemented" error on non-linux hosts
func enableUDSPassCred(conn *net.UnixConn) error {
	return ErrLinuxOnly
}

// processUDSOrigin returns a "not implemented" error on non-linux hosts
func processUDSOrigin(oob []byte) (string, error) {
	return NoOrigin, ErrLinuxOnly
}
