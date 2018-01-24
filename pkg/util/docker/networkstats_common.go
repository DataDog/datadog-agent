// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package docker

// InterfaceNetStats stores network statistics about a Docker network interface
type InterfaceNetStats struct {
	NetworkName string
	BytesSent   uint64
	BytesRcvd   uint64
	PacketsSent uint64
	PacketsRcvd uint64
}

// ContainerNetStats stores network statistics about a Docker container per interface
type ContainerNetStats []*InterfaceNetStats
