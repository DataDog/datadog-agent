// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpusubscriberimpl subscribes to GPU events
package gpusubscriberimpl

// NoopSubscriber is a no-op implementation of the gpusubscriber.Component interface.
type NoopSubscriber struct{}

// GetGPUTags returns an empty map as a no-op implementation.
func (s NoopSubscriber) GetGPUTags() map[int32][]string {
	return map[int32][]string{}
}
