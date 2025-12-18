// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/alert"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/dmd"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/embedding"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/template"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/window"
)

// Pipeline orchestrates the entire drift detection pipeline
type Pipeline struct {
	config common.Config

	// Channels connecting pipeline stages
	logChan       chan common.LogEntry
	windowChan    chan common.Window
	templateChan  chan common.TemplateResult
	embeddingChan chan common.EmbeddingResult
	dmdChan       chan common.DMDResult

	// Pipeline stages
	windowManager     *window.Manager
	templateExtractor *template.Extractor
	embeddingClient   *embedding.Client
	dmdAnalyzer       *dmd.Analyzer
	alertManager      *alert.Manager
}

// New creates a new drift detection pipeline
func New(config common.Config) *Pipeline {
	// Create channels with appropriate buffer sizes
	logChan := make(chan common.LogEntry, 10000) // Large buffer for log ingestion
	windowChan := make(chan common.Window, 10)   // Small buffer for windows
	templateChan := make(chan common.TemplateResult, 10)
	embeddingChan := make(chan common.EmbeddingResult, 10)
	dmdChan := make(chan common.DMDResult, 10)

	// Create pipeline stages
	windowManager := window.NewManager(config.Window, logChan, windowChan)
	templateExtractor := template.NewExtractor(config.Template, windowChan, templateChan)
	embeddingClient := embedding.NewClient(config.Embedding, templateChan, embeddingChan)
	dmdAnalyzer := dmd.NewAnalyzer(config.DMD, embeddingChan, dmdChan)
	alertManager := alert.NewManager(config.Alert, dmdChan, config.Telemetry)

	return &Pipeline{
		config:            config,
		logChan:           logChan,
		windowChan:        windowChan,
		templateChan:      templateChan,
		embeddingChan:     embeddingChan,
		dmdChan:           dmdChan,
		windowManager:     windowManager,
		templateExtractor: templateExtractor,
		embeddingClient:   embeddingClient,
		dmdAnalyzer:       dmdAnalyzer,
		alertManager:      alertManager,
	}
}

// Start starts all pipeline stages
func (p *Pipeline) Start() error {
	// Start all stages in order
	p.windowManager.Start()
	p.templateExtractor.Start()
	p.embeddingClient.Start()
	p.dmdAnalyzer.Start()
	p.alertManager.Start()

	return nil
}

// Stop stops all pipeline stages gracefully
func (p *Pipeline) Stop() {
	// Stop in reverse order to allow graceful shutdown
	p.windowManager.Stop()
	// Wait for remaining data to flow through
	time.Sleep(100 * time.Millisecond)
	p.templateExtractor.Stop()
	time.Sleep(100 * time.Millisecond)
	p.embeddingClient.Stop()
	time.Sleep(100 * time.Millisecond)
	p.dmdAnalyzer.Stop()
	time.Sleep(100 * time.Millisecond)
	p.alertManager.Stop()
}

// ProcessLog adds a log entry to the pipeline
func (p *Pipeline) ProcessLog(timestamp time.Time, content string) {
	select {
	case p.logChan <- common.LogEntry{
		Timestamp: timestamp,
		Content:   content,
	}:
	default:
		// Channel full, drop log (back-pressure)
		// In production, might want to track dropped logs
	}
}

// IsEnabled returns whether the pipeline is enabled
func (p *Pipeline) IsEnabled() bool {
	return p.config.Embedding.Enabled
}
