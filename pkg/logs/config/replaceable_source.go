package config

import (
	"sync"
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
	return &ReplaceableSource{
		source: source,
	}
}

// Replace replaces the source with a new one
func (r *ReplaceableSource) Replace(source *LogSource) {
	r.Lock()
	defer r.Unlock()
	r.source = source
}

// Identifier gets the config identifier
func (r *ReplaceableSource) Identifier() string {
	r.RLock()
	defer r.RUnlock()
	return r.source.Config.Identifier
}

// Status gets the underlying status
func (r *ReplaceableSource) Status() *LogStatus {
	r.RLock()
	defer r.RUnlock()
	return r.source.Status
}

// Config gets the underlying config
func (r *ReplaceableSource) Config() *LogsConfig {
	r.RLock()
	defer r.RUnlock()
	return r.source.Config
}

// AddInput registers an input as being handled by this source.
func (r *ReplaceableSource) AddInput(input string) {
	r.RLock()
	defer r.RUnlock()
	r.source.AddInput(input)
}

// RemoveInput removes an input from this source.
func (r *ReplaceableSource) RemoveInput(input string) {
	r.RLock()
	defer r.RUnlock()
	r.source.RemoveInput(input)
}

// RecordBytes reports bytes to the source expvars
func (r *ReplaceableSource) RecordBytes(n int64) {
	r.RLock()
	defer r.RUnlock()
	r.source.RecordBytes(n)
}

// GetSourceType gets the source type
func (r *ReplaceableSource) GetSourceType() SourceType {
	r.RLock()
	defer r.RUnlock()
	return r.source.sourceType
}

// UnderlyingSource gets the underlying log source
func (r *ReplaceableSource) UnderlyingSource() *LogSource {
	r.RLock()
	defer r.RUnlock()
	return r.source
}
