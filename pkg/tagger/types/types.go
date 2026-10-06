// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types implements the types used by the Tagger for Origin Detection.
package types

import "github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"

// OriginInfo contains the Origin Detection information.
type OriginInfo struct {
	ContainerIDFromSocket string                        // ContainerIDFromSocket is the origin resolved using Unix Domain Socket.
	LocalData             origindetection.LocalData     // LocalData is the local data list.
	ExternalData          origindetection.ExternalData  // ExternalData is the external data list.
	Cardinality           string                        // Cardinality is the cardinality of the resolved origin.
	ProductOrigin         origindetection.ProductOrigin // ProductOrigin is the product that sent the origin information.
	// Resolved, when non-nil, holds container IDs the producer already resolved from the inode
	// or the external data, so that the tagger does not resolve them a second time.
	// It must be treated as read-only since it can be shared across many samples.
	Resolved *ResolvedOrigin
}

// ResolvedOrigin holds the container IDs resolved from origin detection data.
// A resolution is only reused when its "Done" field is set, which allows storing
// failed resolutions (empty container ID) without retrying them.
type ResolvedOrigin struct {
	InodeContainerID        string // InodeContainerID is the container ID resolved from LocalData.Inode.
	InodeDone               bool   // InodeDone is true when the inode resolution was attempted.
	ExternalDataContainerID string // ExternalDataContainerID is the container ID resolved from ExternalData.
	ExternalDataDone        bool   // ExternalDataDone is true when the external data resolution was attempted.
}
