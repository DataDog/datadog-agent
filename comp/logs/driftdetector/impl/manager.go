// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/alert"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/dmd"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/embedding"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/template"
	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/window"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// driftDetectorManager manages shared components and per-source pipelines
type driftDetectorManager struct {
	config  common.Config
	enabled bool

	mu         sync.RWMutex
	pipelines  map[string]*perSourcePipeline // sourceKey -> simplified pipeline
	lastAccess map[string]time.Time

	// Shared components (ONE instance for all sources)
	sharedWindowManager     *window.Manager
	sharedTemplateExtractor *template.Extractor
	sharedAlertManager      *alert.Manager

	// Shared HTTP transport for embedding clients
	sharedHTTPTransport *http.Transport

	// Shared channels
	logChan      chan common.LogEntry
	windowChan   chan common.Window
	templateChan chan common.TemplateResult
	dmdChan      chan common.DMDResult

	// Cleanup
	cleanupInterval time.Duration
	maxIdleTime     time.Duration
	stopCleanup     chan struct{}
}

// perSourcePipeline is a lightweight per-source pipeline
// It only contains per-source components: EmbeddingClient + DMDAnalyzer
type perSourcePipeline struct {
	sourceKey string

	embeddingClient *embedding.Client
	dmdAnalyzer     *dmd.Analyzer

	// Channels
	templateFilterChan chan common.TemplateResult
	embeddingChan      chan common.EmbeddingResult
	dmdChan            chan common.DMDResult

	// Routing goroutines contexts
	ctx    context.Context
	cancel context.CancelFunc
}

// newDriftDetectorManager creates a new manager with shared components
func newDriftDetectorManager(config common.Config) *driftDetectorManager {
	// Create shared channels with larger buffers for all sources
	logChan := make(chan common.LogEntry, 10000)
	windowChan := make(chan common.Window, 100)
	templateChan := make(chan common.TemplateResult, 100)
	dmdChan := make(chan common.DMDResult, 100)

	// Create shared HTTP transport (ONE connection pool for all embedding clients)
	sharedHTTPTransport := &http.Transport{
		MaxIdleConns:        10, // Total for ALL sources
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}

	// Create shared components ONCE
	sharedWindowManager := window.NewManager(config.Window, logChan, windowChan)
	sharedTemplateExtractor := template.NewExtractor(config.Template, windowChan, templateChan)
	sharedAlertManager := alert.NewManager(config.Alert, dmdChan, config.Telemetry)

	return &driftDetectorManager{
		config:                  config,
		enabled:                 config.Embedding.Enabled,
		pipelines:               make(map[string]*perSourcePipeline),
		lastAccess:              make(map[string]time.Time),
		sharedWindowManager:     sharedWindowManager,
		sharedTemplateExtractor: sharedTemplateExtractor,
		sharedAlertManager:      sharedAlertManager,
		sharedHTTPTransport:     sharedHTTPTransport,
		logChan:                 logChan,
		windowChan:              windowChan,
		templateChan:            templateChan,
		dmdChan:                 dmdChan,
		cleanupInterval:         config.Manager.CleanupInterval,
		maxIdleTime:             config.Manager.MaxIdleTime,
		stopCleanup:             make(chan struct{}),
	}
}

// Start starts the manager and shared components
func (m *driftDetectorManager) Start() error {
	if !m.enabled {
		log.Info("Drift detector manager is disabled")
		return nil
	}

	log.Info("Starting drift detector manager with shared components")

	// Start shared components ONCE
	m.sharedWindowManager.Start()
	m.sharedTemplateExtractor.Start()
	m.sharedAlertManager.Start()

	// Start cleanup routine
	go m.cleanupRoutine()

	return nil
}

// Stop stops all pipelines and shared components
func (m *driftDetectorManager) Stop() {
	if !m.enabled {
		return
	}

	log.Info("Stopping drift detector manager")
	close(m.stopCleanup)

	// Stop all per-source pipelines first
	m.mu.Lock()
	pipelinesToStop := make([]*perSourcePipeline, 0, len(m.pipelines))
	for _, p := range m.pipelines {
		pipelinesToStop = append(pipelinesToStop, p)
	}
	m.pipelines = make(map[string]*perSourcePipeline)
	m.lastAccess = make(map[string]time.Time)
	m.mu.Unlock()

	for _, p := range pipelinesToStop {
		p.Stop()
	}

	// Stop shared components LAST (after all sources stopped)
	m.sharedWindowManager.Stop()
	time.Sleep(50 * time.Millisecond)
	m.sharedTemplateExtractor.Stop()
	time.Sleep(50 * time.Millisecond)
	m.sharedAlertManager.Stop()

	// Close shared HTTP transport
	m.sharedHTTPTransport.CloseIdleConnections()
}

// ProcessLog sends a log to the shared window manager with source identification
func (m *driftDetectorManager) ProcessLog(sourceKey string, timestamp time.Time, content string) {
	if !m.enabled {
		return
	}

	// Update last access time
	m.mu.Lock()
	m.lastAccess[sourceKey] = time.Now()
	m.mu.Unlock()

	// Ensure pipeline exists for this source (creates per-source embedding+DMD)
	m.getOrCreatePipeline(sourceKey)

	// Send log to shared window manager (it will handle per-source windowing)
	m.sharedWindowManager.ProcessLog(sourceKey, timestamp, content)
}

// getOrCreatePipeline gets an existing pipeline or creates a new one for the source
func (m *driftDetectorManager) getOrCreatePipeline(sourceKey string) *perSourcePipeline {
	// Try to get existing pipeline (fast path with read lock)
	m.mu.RLock()
	pipeline, exists := m.pipelines[sourceKey]
	m.mu.RUnlock()

	if exists {
		return pipeline
	}

	// Create new pipeline (slow path with write lock)
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check in case another goroutine created it
	pipeline, exists = m.pipelines[sourceKey]
	if exists {
		return pipeline
	}

	// Create new per-source pipeline
	log.Infof("Creating drift detector pipeline for source: %s", sourceKey)
	pipeline = newPerSourcePipeline(sourceKey, m.config, m.templateChan, m.dmdChan, m.sharedHTTPTransport)

	// Start the pipeline
	if err := pipeline.Start(); err != nil {
		log.Errorf("Failed to start pipeline for source %s: %v", sourceKey, err)
		return nil
	}

	// Store pipeline
	m.pipelines[sourceKey] = pipeline
	m.lastAccess[sourceKey] = time.Now()

	log.Infof("Drift detector pipeline created for source: %s (total pipelines: %d)", sourceKey, len(m.pipelines))
	return pipeline
}

// removePipeline removes and stops a pipeline for a source
func (m *driftDetectorManager) removePipeline(sourceKey string) {
	m.mu.Lock()
	pipeline, exists := m.pipelines[sourceKey]
	if !exists {
		m.mu.Unlock()
		return
	}

	delete(m.pipelines, sourceKey)
	delete(m.lastAccess, sourceKey)
	remainingCount := len(m.pipelines)
	m.mu.Unlock()

	log.Infof("Removing idle drift detector pipeline for source: %s", sourceKey)
	pipeline.Stop()
	log.Infof("Drift detector pipeline removed (remaining pipelines: %d)", remainingCount)
}

// cleanupRoutine periodically removes idle pipelines
func (m *driftDetectorManager) cleanupRoutine() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.stopCleanup:
			return
		case <-ticker.C:
			m.cleanupIdlePipelines()
		}
	}
}

// cleanupIdlePipelines removes pipelines that haven't been used recently
func (m *driftDetectorManager) cleanupIdlePipelines() {
	m.mu.RLock()
	now := time.Now()
	toRemove := []string{}

	for sourceKey, lastAccess := range m.lastAccess {
		if now.Sub(lastAccess) > m.maxIdleTime {
			toRemove = append(toRemove, sourceKey)
		}
	}
	m.mu.RUnlock()

	// Remove idle pipelines
	for _, sourceKey := range toRemove {
		m.removePipeline(sourceKey)
	}

	if len(toRemove) > 0 {
		log.Infof("Cleaned up %d idle drift detector pipelines", len(toRemove))
	}
}

// GetStats returns statistics about active pipelines
func (m *driftDetectorManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return map[string]interface{}{
		"enabled":          m.enabled,
		"active_pipelines": len(m.pipelines),
		"sources":          m.getSourceKeys(),
	}
}

func (m *driftDetectorManager) getSourceKeys() []string {
	keys := make([]string, 0, len(m.pipelines))
	for k := range m.pipelines {
		keys = append(keys, k)
	}
	return keys
}

// IsEnabled returns whether the manager is enabled
func (m *driftDetectorManager) IsEnabled() bool {
	return m.enabled
}

// newPerSourcePipeline creates a new per-source pipeline with embedding + DMD
func newPerSourcePipeline(sourceKey string, config common.Config,
	templateChan chan common.TemplateResult,
	dmdChan chan common.DMDResult,
	sharedHTTPTransport *http.Transport) *perSourcePipeline {

	ctx, cancel := context.WithCancel(context.Background())

	templateFilterChan := make(chan common.TemplateResult, 10)
	embeddingChan := make(chan common.EmbeddingResult, 10)
	localDMDChan := make(chan common.DMDResult, 10)

	// Create per-source embedding client (reuses shared HTTP transport)
	embeddingClient := embedding.NewClientWithTransport(config.Embedding,
		templateFilterChan, embeddingChan, sharedHTTPTransport)

	// Create per-source DMD analyzer
	dmdAnalyzer := dmd.NewAnalyzer(sourceKey, config.DMD, embeddingChan, localDMDChan)

	pipeline := &perSourcePipeline{
		sourceKey:          sourceKey,
		embeddingClient:    embeddingClient,
		dmdAnalyzer:        dmdAnalyzer,
		templateFilterChan: templateFilterChan,
		embeddingChan:      embeddingChan,
		dmdChan:            localDMDChan,
		ctx:                ctx,
		cancel:             cancel,
	}

	// Start routing goroutines
	// 1. Filter templates from shared extractor by source key
	go pipeline.routeTemplates(templateChan)

	// 2. Route DMD results to shared alert manager
	go pipeline.routeDMDResults(dmdChan)

	return pipeline
}

// Start starts the per-source pipeline components
func (p *perSourcePipeline) Start() error {
	p.embeddingClient.Start()
	p.dmdAnalyzer.Start()
	return nil
}

// Stop stops the per-source pipeline
func (p *perSourcePipeline) Stop() {
	p.cancel() // Stop routing goroutines
	p.embeddingClient.Stop()
	time.Sleep(50 * time.Millisecond)
	p.dmdAnalyzer.Stop()
}

// routeTemplates filters templates from shared extractor for this source
func (p *perSourcePipeline) routeTemplates(templateChan chan common.TemplateResult) {
	for {
		select {
		case <-p.ctx.Done():
			close(p.templateFilterChan)
			return

		case template, ok := <-templateChan:
			if !ok {
				close(p.templateFilterChan)
				return
			}

			// Filter: Only pass templates for MY source
			if template.SourceKey == p.sourceKey {
				select {
				case p.templateFilterChan <- template:
				case <-p.ctx.Done():
					close(p.templateFilterChan)
					return
				}
			}
		}
	}
}

// routeDMDResults routes DMD results to shared alert manager
func (p *perSourcePipeline) routeDMDResults(dmdChan chan common.DMDResult) {
	for {
		select {
		case <-p.ctx.Done():
			return

		case result, ok := <-p.dmdChan:
			if !ok {
				return
			}

			// Forward to shared alert manager
			select {
			case dmdChan <- result:
			case <-p.ctx.Done():
				return
			}
		}
	}
}
