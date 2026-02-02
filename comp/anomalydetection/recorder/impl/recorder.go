// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package recorderimpl implements the recorder component interface
package recorderimpl

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provides defines the output of the recorder component
type Provides struct {
	Comp recorderdef.Component
}

// NewComponent creates a new recorder component
func NewComponent() (Provides, error) {
	r := &recorderImpl{
		handles: make(map[string]observer.Handle),
	}
	r.mode.Store(int32(recorderdef.ModePassthrough))

	// Check configuration to start recording automatically if configured
	cfg := pkgconfigsetup.Datadog()

	// Initialize recording if both capture_metrics.enabled AND parquet_output_dir are configured.
	captureMetricsEnabled := cfg.GetBool("observer.capture_metrics.enabled")
	if captureMetricsEnabled {
		if parquetDir := cfg.GetString("observer.parquet_output_dir"); parquetDir != "" {
			flushInterval := cfg.GetDuration("observer.parquet_flush_interval")
			if flushInterval == 0 {
				flushInterval = 60 * time.Second
			}

			retentionDuration := cfg.GetDuration("observer.parquet_retention")
			if retentionDuration <= 0 {
				retentionDuration = 24 * time.Hour
			}

			// Start recording automatically based on config
			err := r.StartRecording(recorderdef.RecordingConfig{
				OutputDir:         parquetDir,
				FlushInterval:     flushInterval,
				RetentionDuration: retentionDuration,
			})
			if err != nil {
				pkglog.Errorf("Failed to start recorder with config: %v", err)
			} else {
				pkglog.Infof("Recorder started with parquet output: dir=%s flush=%v retention=%v", parquetDir, flushInterval, retentionDuration)
			}
		}
	} else {
		pkglog.Debug("Recorder parquet writer disabled (observer.capture_metrics.enabled is false)")
	}

	return Provides{Comp: r}, nil
}

// recorderImpl implements the recorder component
type recorderImpl struct {
	mode atomic.Int32 // recorderdef.Mode

	// Recording state
	parquetWriter *ParquetWriter
	recordingMu   sync.Mutex

	// Replay state
	replayCtx    context.Context
	replayCancel context.CancelFunc
	replayMu     sync.Mutex

	// Handle tracking for replay injection
	handles   map[string]observer.Handle
	handlesMu sync.RWMutex
}

// GetHandle wraps the provided HandleFunc with recording/replay capability.
func (r *recorderImpl) GetHandle(handleFunc observer.HandleFunc) observer.HandleFunc {
	return func(name string) observer.Handle {
		innerHandle := handleFunc(name)

		// Store handle for potential replay injection
		r.handlesMu.Lock()
		r.handles[name] = innerHandle
		r.handlesMu.Unlock()

		mode := recorderdef.Mode(r.mode.Load())
		switch mode {
		case recorderdef.ModeRecording:
			return &recordingHandle{
				inner:    innerHandle,
				recorder: r,
				name:     name,
			}
		case recorderdef.ModeReplaying:
			// In replay mode, we pass through but the replay goroutine injects metrics
			return innerHandle
		default:
			// Passthrough mode
			return innerHandle
		}
	}
}

// StartRecording starts recording observations to parquet files.
func (r *recorderImpl) StartRecording(config recorderdef.RecordingConfig) error {
	r.recordingMu.Lock()
	defer r.recordingMu.Unlock()

	currentMode := recorderdef.Mode(r.mode.Load())
	if currentMode == recorderdef.ModeRecording {
		return fmt.Errorf("recording already active")
	}
	if currentMode == recorderdef.ModeReplaying {
		return fmt.Errorf("cannot start recording while replaying")
	}

	// Apply defaults
	if config.FlushInterval == 0 {
		config.FlushInterval = 60 * time.Second
	}
	if config.RetentionDuration == 0 {
		config.RetentionDuration = 24 * time.Hour
	}

	// Create parquet writer
	writer, err := NewParquetWriter(config.OutputDir, config.FlushInterval, config.RetentionDuration)
	if err != nil {
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	r.parquetWriter = writer
	r.mode.Store(int32(recorderdef.ModeRecording))

	pkglog.Infof("Recording started: dir=%s flush=%v retention=%v", config.OutputDir, config.FlushInterval, config.RetentionDuration)
	return nil
}

// StopRecording stops the current recording session.
func (r *recorderImpl) StopRecording() error {
	r.recordingMu.Lock()
	defer r.recordingMu.Unlock()

	if recorderdef.Mode(r.mode.Load()) != recorderdef.ModeRecording {
		return fmt.Errorf("not currently recording")
	}

	if r.parquetWriter != nil {
		if err := r.parquetWriter.Close(); err != nil {
			pkglog.Errorf("Error closing parquet writer: %v", err)
		}
		r.parquetWriter = nil
	}

	r.mode.Store(int32(recorderdef.ModePassthrough))
	pkglog.Info("Recording stopped")
	return nil
}

// IsRecording returns true if recording is currently active.
func (r *recorderImpl) IsRecording() bool {
	return recorderdef.Mode(r.mode.Load()) == recorderdef.ModeRecording
}

// StartReplay starts replaying observations from parquet files.
func (r *recorderImpl) StartReplay(config recorderdef.PlaybackConfig) error {
	r.replayMu.Lock()
	defer r.replayMu.Unlock()

	currentMode := recorderdef.Mode(r.mode.Load())
	if currentMode == recorderdef.ModeReplaying {
		return fmt.Errorf("replay already active")
	}
	if currentMode == recorderdef.ModeRecording {
		return fmt.Errorf("cannot start replay while recording")
	}

	// Apply defaults
	if config.TimeScale <= 0 {
		config.TimeScale = 1.0
	}

	// Get a handle to inject metrics into
	r.handlesMu.RLock()
	var replayHandle observer.Handle
	for _, h := range r.handles {
		replayHandle = h
		break
	}
	r.handlesMu.RUnlock()

	if replayHandle == nil {
		return fmt.Errorf("no handles available for replay injection")
	}

	// Create replay context
	ctx, cancel := context.WithCancel(context.Background())
	r.replayCtx = ctx
	r.replayCancel = cancel

	// Start replay goroutine
	go func() {
		generator, err := NewParquetReplayGenerator(replayHandle, ParquetReplayConfig{
			ParquetDir: config.InputDir,
			TimeScale:  config.TimeScale,
			Loop:       config.Loop,
		})
		if err != nil {
			pkglog.Errorf("Failed to create replay generator: %v", err)
			r.mode.Store(int32(recorderdef.ModePassthrough))
			return
		}

		generator.Run(ctx)

		// Reset mode when replay completes
		r.replayMu.Lock()
		r.mode.Store(int32(recorderdef.ModePassthrough))
		r.replayMu.Unlock()
		pkglog.Info("Replay completed")
	}()

	r.mode.Store(int32(recorderdef.ModeReplaying))
	pkglog.Infof("Replay started: dir=%s timescale=%.2f loop=%v", config.InputDir, config.TimeScale, config.Loop)
	return nil
}

// StopReplay stops the current replay session.
func (r *recorderImpl) StopReplay() error {
	r.replayMu.Lock()
	defer r.replayMu.Unlock()

	if recorderdef.Mode(r.mode.Load()) != recorderdef.ModeReplaying {
		return fmt.Errorf("not currently replaying")
	}

	if r.replayCancel != nil {
		r.replayCancel()
		r.replayCancel = nil
		r.replayCtx = nil
	}

	r.mode.Store(int32(recorderdef.ModePassthrough))
	pkglog.Info("Replay stopped")
	return nil
}

// IsReplaying returns true if replay is currently active.
func (r *recorderImpl) IsReplaying() bool {
	return recorderdef.Mode(r.mode.Load()) == recorderdef.ModeReplaying
}

// Mode returns the current operating mode.
func (r *recorderImpl) Mode() recorderdef.Mode {
	return recorderdef.Mode(r.mode.Load())
}

// recordingHandle wraps an observer handle to record observations.
type recordingHandle struct {
	inner    observer.Handle
	recorder *recorderImpl
	name     string
}

// ObserveMetric forwards the metric to the inner handle and records it.
func (h *recordingHandle) ObserveMetric(sample observer.MetricView) {
	// Forward to inner handle first
	h.inner.ObserveMetric(sample)

	// Record to parquet if writer is available
	if h.recorder.parquetWriter != nil {
		timestamp := int64(sample.GetTimestamp())
		if timestamp == 0 {
			timestamp = time.Now().Unix()
		}
		h.recorder.parquetWriter.WriteMetric(
			h.name,
			sample.GetName(),
			sample.GetValue(),
			sample.GetRawTags(),
			timestamp,
		)
	}
}

// ObserveLog forwards the log to the inner handle.
// Log recording is not implemented yet but the hook is in place.
func (h *recordingHandle) ObserveLog(msg observer.LogView) {
	h.inner.ObserveLog(msg)
	// TODO: Optionally record logs to parquet (future enhancement)
}
