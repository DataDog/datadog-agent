// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorder provides a middleware component for recording and replaying observer data.
//
// The recorder intercepts metrics and logs flowing through observer handles, optionally
// recording them to parquet files for later replay. This enables:
// - Capturing production data for offline analysis
// - Replaying recorded data for testing and debugging
// - Building reproducible test scenarios from real workloads
package recorder

import (
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// team: agent-metric-pipelines

// Component is the recorder middleware component.
// It wraps observer handles to intercept and optionally record observations.
type Component interface {
	// GetHandle wraps the provided HandleFunc with recording/replay capability.
	// This is called by the observer's GetHandle to create the final handle chain.
	GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc

	// StartRecording starts recording observations to parquet files.
	// Returns an error if recording is already active or if the output directory is invalid.
	StartRecording(config RecordingConfig) error

	// StopRecording stops the current recording session.
	// This flushes any buffered data and closes the parquet files.
	StopRecording() error

	// IsRecording returns true if recording is currently active.
	IsRecording() bool

	// StartReplay starts replaying observations from parquet files.
	// The replay injects metrics into the observer as if they were live data.
	// Returns an error if replay is already active or if the input directory is invalid.
	StartReplay(config PlaybackConfig) error

	// StopReplay stops the current replay session.
	StopReplay() error

	// IsReplaying returns true if replay is currently active.
	IsReplaying() bool

	// Mode returns the current operating mode.
	Mode() Mode
}

// Mode represents the recorder's operating mode.
type Mode int

const (
	// ModePassthrough just forwards observations to the inner handle without recording.
	ModePassthrough Mode = iota
	// ModeRecording forwards observations and also writes them to parquet files.
	ModeRecording
	// ModeReplaying reads observations from parquet files and injects them.
	ModeReplaying
)

// String returns a human-readable name for the mode.
func (m Mode) String() string {
	switch m {
	case ModePassthrough:
		return "passthrough"
	case ModeRecording:
		return "recording"
	case ModeReplaying:
		return "replaying"
	default:
		return "unknown"
	}
}

// RecordingConfig configures how observations are recorded to parquet files.
type RecordingConfig struct {
	// OutputDir is the directory where parquet files will be written.
	OutputDir string

	// FlushInterval is how often to flush buffered data to disk.
	// Shorter intervals provide more frequent checkpoints but higher I/O.
	// Default: 60 seconds.
	FlushInterval time.Duration

	// RetentionDuration is how long to keep old parquet files.
	// Files older than this will be automatically deleted.
	// Default: 24 hours. Set to 0 to disable cleanup.
	RetentionDuration time.Duration
}

// PlaybackConfig configures how observations are replayed from parquet files.
type PlaybackConfig struct {
	// InputDir is the directory containing parquet files to replay.
	InputDir string

	// TimeScale controls replay speed.
	// 1.0 = realtime, 0.5 = 2x faster, 2.0 = 2x slower.
	// Default: 1.0.
	TimeScale float64

	// Loop controls whether to loop the replay after reaching the end.
	// Default: false.
	Loop bool

	// StartTime is the timestamp (unix seconds) to start replay from.
	// Default: 0 (start from beginning).
	StartTime int64

	// EndTime is the timestamp (unix seconds) to stop replay at.
	// Default: 0 (replay until end).
	EndTime int64
}
