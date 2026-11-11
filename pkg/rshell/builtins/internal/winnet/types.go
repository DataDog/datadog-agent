// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package winnet provides socket enumeration for Windows via iphlpapi.dll.
// It is placed in builtins/internal/ to isolate the unsafe.Pointer DLL call
// from the analysis checker, which cannot evaluate build tags.
package winnet

// SocketEntry holds the parsed fields for one Windows socket.
type SocketEntry struct {
	Proto      string // "tcp4", "tcp6", "udp4", "udp6"
	State      string // human-readable state name
	LocalIP    string
	LocalPort  uint16
	RemoteIP   string
	RemotePort uint16
}
