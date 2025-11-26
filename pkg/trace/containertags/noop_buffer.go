// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containertagsbuffer contains the logic to buffer payloads for container tags
// enrichment
package containertagsbuffer

// NoOpTagsBuffer is a tagsbuffer that does nothing
type NoOpTagsBuffer struct{}

// Start does nothing
func (n *NoOpTagsBuffer) Start() {}

// Stop does nothing
func (n *NoOpTagsBuffer) Stop() {}

// IsEnabled returns false
func (n *NoOpTagsBuffer) IsEnabled() bool { return false }

// AsyncEnrichment returns false as no enrichment is pending
func (n *NoOpTagsBuffer) AsyncEnrichment(string, func([]string, error), int64) bool {
	return false
}
