// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux && !darwin && !windows

package guiimpl

import (
	"errors"
	"net"
)

// lookupLoopbackPeerIdentity is unimplemented here; callers fall back to the pre-existing TTL/single-use protection.
func lookupLoopbackPeerIdentity(_ net.IP, _, _ int, _ net.IP) (peerIdentity, error) {
	return "", errors.New("peer identity resolution is not implemented on this platform")
}

// elevatedMintIdentity binds to root here too, since lookupLoopbackPeerIdentity never resolves anything on this platform.
func elevatedMintIdentity() peerIdentity {
	return rootIdentity
}
