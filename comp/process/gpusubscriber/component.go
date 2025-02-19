// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpusubscriber implements a component to subscribe to WorkloadMeta in the Agent.
package gpusubscriber

// team: container-intake

type Component interface {
	GetGPUTags() map[int32][]string
}
