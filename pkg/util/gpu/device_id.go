// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gpu

import (
	"regexp"
	"strconv"
)

// nvidiaReplicaSuffixRegex matches the replica suffix of a shared NVIDIA
// device ID. When the NVIDIA device plugin shares a device by replication, as
// time-slicing and MPS do, it registers one device per replica whose ID is
// the base device ID followed by "::<replica index>":
//
//	GPU-170ab964-06dc-a639-af22-0cafd46259c7::2
//
// The scheme is a device-plugin convention (internal/rm/devices.go
// NewAnnotatedID), not a stable API: it is independent of the resource name,
// and its format may change. Parse defensively: an unrecognized shape means
// "not a replica", never an error.
//
// The "GPU-…"/"MIG-…" prefixes of the base ID are NVML's documented UUID
// scheme (MIG user guide, NVML changelog, and the plugin's own
// api/config/v1/replicas.go), always uppercase. Matching is case-sensitive
// end to end — the device UUID matcher compares NVML's UUIDs exactly — so
// an unexpected case surfaces as an attribution error, never a wrong match.
var nvidiaReplicaSuffixRegex = regexp.MustCompile(`^(GPU-|MIG-)(.+)::(\d+)$`)

// NvidiaDeviceReplica is a shared-device replica allocation: one of the
// replicas the NVIDIA device plugin registered of a base device.
type NvidiaDeviceReplica struct {
	// BaseDeviceID is the base device ID without the replica suffix: the
	// UUID of the physical GPU, or of the MIG instance when the shared
	// device is a MIG one.
	BaseDeviceID string
	// Replica is the index of the replica, from the device ID suffix.
	Replica int
}

// ParseNvidiaDeviceReplica parses an NVIDIA device-plugin ID carrying a
// replica suffix, e.g. "GPU-<uuid>::2", into its base device ID and replica
// index. It reports false when the ID carries no suffix or does not match
// the expected format, so an unrecognized shape means "not a replica",
// never an error.
func ParseNvidiaDeviceReplica(deviceID string) (NvidiaDeviceReplica, bool) {
	match := nvidiaReplicaSuffixRegex.FindStringSubmatch(deviceID)
	if match == nil {
		return NvidiaDeviceReplica{}, false
	}
	// The regex only matches decimal digits, so this cannot fail; kept
	// defensive anyway so a regex edit cannot turn into a panic.
	replica, err := strconv.Atoi(match[3])
	if err != nil {
		return NvidiaDeviceReplica{}, false
	}
	return NvidiaDeviceReplica{
		BaseDeviceID: match[1] + match[2],
		Replica:      replica,
	}, true
}
