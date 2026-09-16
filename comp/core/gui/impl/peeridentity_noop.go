// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package guiimpl

import (
	"fmt"
	"net"
)

// lookupLoopbackPeerIdentity is not implemented on this platform; callers fall back to the pre-existing TTL/single-use protection instead.
func lookupLoopbackPeerIdentity(_ net.IP, _, _ int, _ net.IP) (peerIdentity, error) {
	return "", fmt.Errorf("peer identity resolution is not implemented on this platform")
}
