// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpusubscriber subscribes to GPU events
package gpusubscriber

// team: container-experiences

// Component is the component type.
type Component interface {
	// GetGPUTags returns a map of PIDs to their corresponding GPU tags.
	GetGPUTags() map[int32][]string
}
