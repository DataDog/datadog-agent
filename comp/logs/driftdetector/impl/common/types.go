// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

// LogEntry represents a single log line with timestamp and content
type LogEntry struct {
	SourceKey string // Source identifier for routing
	Timestamp time.Time
	Content   string
}

// Window represents a collection of logs within a time window
type Window struct {
	SourceKey string // Source identifier for routing
	ID        int
	StartTime time.Time
	EndTime   time.Time
	Logs      []LogEntry
}

// TemplateResult represents extracted templates from a window
type TemplateResult struct {
	SourceKey        string // Source identifier for routing
	WindowID         int
	Templates        []string
	CompressionRatio float64
}

// Vector represents an embedding vector (dimensions depend on model: 384 for all-MiniLM-L6-v2, 768 for embeddinggemma)
type Vector []float64

// EmbeddingResult represents embeddings for templates
type EmbeddingResult struct {
	WindowID   int
	Templates  []string
	Embeddings []Vector
}

// DMDResult represents the DMD analysis result
type DMDResult struct {
	SourceKey           string // Source identifier for logging
	WindowID            int
	ReconstructionError float64
	NormalizedError     float64
	Templates           []string
}

// Alert represents an anomaly detection alert
type Alert struct {
	SourceKey           string // Source identifier for logging
	Timestamp           time.Time
	WindowID            int
	ReconstructionError float64
	NormalizedError     float64
	Severity            Severity
	Templates           []string
}

// Severity represents the alert severity level
type Severity string

const (
	// SeverityWarning indicates a 2-sigma deviation
	SeverityWarning Severity = "warning"
	// SeverityCritical indicates a 3-sigma deviation
	SeverityCritical Severity = "critical"
)

// Config holds all configuration for the drift detection pipeline
type Config struct {
	Window    WindowConfig
	Template  TemplateConfig
	Embedding EmbeddingConfig
	DMD       DMDConfig
	Alert     AlertConfig
	Manager   ManagerConfig
	Telemetry telemetry.Component
}

// WindowConfig configures the sliding window behavior
type WindowConfig struct {
	Size time.Duration // Window size (default: 120s)
	Step time.Duration // Step size for sliding (default: 60s)
}

// TemplateConfig configures template extraction
type TemplateConfig struct {
	EntropyThreshold         float64 // Threshold for variable detection (default: 2.5)
	EnumCardinalityThreshold int     // Max cardinality for enum detection (default: 10)
	MaxCharacters            int     // Max characters per template for embedding (default: 8000)
	WorkerCount              int     // Number of parallel extraction workers (default: 4)
}

// EmbeddingConfig configures the embedding service client
type EmbeddingConfig struct {
	ServerURL      string        // Embedding service URL
	Model          string        // Model name (default: "embeddinggemma")
	BatchSize      int           // Max batch size (default: 100)
	BatchTimeout   time.Duration // Max wait before flushing batch (default: 5s)
	MaxRetries     int           // Max retry attempts (default: 3)
	Timeout        time.Duration // HTTP request timeout (default: 30s)
	MaxConnections int           // Connection pool size (default: 10)
	Enabled        bool          // Enable/disable drift detection (default: false)
}

// ManagerConfig configures the drift detector manager
type ManagerConfig struct {
	CleanupInterval time.Duration // How often to check for idle detectors (default: 5m)
	MaxIdleTime     time.Duration // Max idle time before removing detector (default: 30m)
}

// DMDConfig configures the DMD analysis
type DMDConfig struct {
	TimeDelay       int           // Hankel time delay depth (default: 5)
	WindowRetention time.Duration // How long to retain windows (default: 2h)
	RecomputeEvery  int           // Recompute DMD every N windows (default: 10)
	Rank            int           // SVD rank for dimensionality reduction (default: 50)
}

// AlertConfig configures anomaly detection thresholds
type AlertConfig struct {
	WarningThreshold  float64 // Standard deviations for warning (default: 2.0)
	CriticalThreshold float64 // Standard deviations for critical (default: 3.0)
}

// NewDefaultConfig returns a Config with default values
func NewDefaultConfig() Config {
	return Config{
		Window: WindowConfig{
			Size: 120 * time.Second,
			Step: 60 * time.Second,
		},
		Template: TemplateConfig{
			EntropyThreshold:         2.5,
			EnumCardinalityThreshold: 10,
			MaxCharacters:            8000,
			WorkerCount:              4,
		},
		Embedding: EmbeddingConfig{
			ServerURL:      "http://localhost:11434/api/embed",
			Model:          "embeddinggemma",
			BatchSize:      100,
			BatchTimeout:   5 * time.Second,
			MaxRetries:     3,
			Timeout:        30 * time.Second,
			MaxConnections: 10,
			Enabled:        false,
		},
		DMD: DMDConfig{
			TimeDelay:       5,
			WindowRetention: 2 * time.Hour,
			RecomputeEvery:  10,
			Rank:            50,
		},
		Alert: AlertConfig{
			WarningThreshold:  2.0,
			CriticalThreshold: 3.0,
		},
		Manager: ManagerConfig{
			CleanupInterval: 5 * time.Minute,
			MaxIdleTime:     30 * time.Minute,
		},
	}
}
