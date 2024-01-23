// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package sources

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/status"
)

// ReplaceableSource is a thread safe wrapper for a LogSource that allows it to be replaced with a new one.
// There are some uncommon circumstances where a source needs to be replaced on an active tailer. This wrapper
// helps ensure there is not any unsafe access to the many underlying properties in a LogSource.
type ReplaceableSource struct {
	sync.RWMutex
	source *LogSource
}

// NewReplaceableSource returns a new ReplaceableSource
func NewReplaceableSource(source *LogSource) *ReplaceableSource {
	panic("not called")
}

// Replace replaces the source with a new one
func (r *ReplaceableSource) Replace(source *LogSource) {
	panic("not called")
}

// Status gets the underlying status
func (r *ReplaceableSource) Status() *status.LogStatus {
	panic("not called")
}

// Config gets the underlying config
func (r *ReplaceableSource) Config() *config.LogsConfig {
	panic("not called")
}

// AddInput registers an input as being handled by this source.
func (r *ReplaceableSource) AddInput(input string) {
	panic("not called")
}

// RemoveInput removes an input from this source.
func (r *ReplaceableSource) RemoveInput(input string) {
	panic("not called")
}

// RecordBytes reports bytes to the source expvars
func (r *ReplaceableSource) RecordBytes(n int64) {
	panic("not called")
}

// GetSourceType gets the source type
func (r *ReplaceableSource) GetSourceType() SourceType {
	panic("not called")
}

// UnderlyingSource gets the underlying log source
func (r *ReplaceableSource) UnderlyingSource() *LogSource {
	panic("not called")
}

// RegisterInfo registers some info to display on the status page
func (r *ReplaceableSource) RegisterInfo(i status.InfoProvider) {
	panic("not called")
}

// GetInfo gets an InfoProvider instance by the key
func (r *ReplaceableSource) GetInfo(key string) status.InfoProvider {
	panic("not called")
}
