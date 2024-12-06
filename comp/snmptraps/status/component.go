// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package status exposes the expvars we use for status tracking to the
// component system.
package status

// team: ndm-core

// Component is the component type.
type Component interface {
	AddTrapsPackets(int64)
	GetTrapsPackets() int64
	AddTrapsPacketsUnknownCommunityString(int64)
	GetTrapsPacketsUnknownCommunityString() int64
	SetStartError(error)
	GetStartError() error
}
