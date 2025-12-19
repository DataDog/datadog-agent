// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// embedRequest represents the HTTP request to the embedding service (Ollama format)
type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"` // Ollama expects array of strings
}

// embedResponse represents the HTTP response from the embedding service (Ollama format)
type embedResponse struct {
	Embeddings [][]float64 `json:"embeddings"` // Ollama returns direct array of embeddings
	Model      string      `json:"model,omitempty"`
}

// Client handles communication with the embedding service
type Client struct {
	config     common.EmbeddingConfig
	httpClient *http.Client
	inputChan  chan common.TemplateResult
	outputChan chan common.EmbeddingResult
	ctx        context.Context
	cancel     context.CancelFunc

	// Batching state
	batch          []batchItem
	batchTimer     *time.Timer
	batchTimerDone chan struct{}
}

type batchItem struct {
	windowID  int
	templates []string
}

// NewClient creates a new embedding client with its own HTTP transport
func NewClient(config common.EmbeddingConfig, inputChan chan common.TemplateResult, outputChan chan common.EmbeddingResult) *Client {
	// Create own HTTP transport
	transport := &http.Transport{
		MaxIdleConns:        config.MaxConnections,
		MaxIdleConnsPerHost: config.MaxConnections,
		IdleConnTimeout:     90 * time.Second,
	}
	return NewClientWithTransport(config, inputChan, outputChan, transport)
}

// NewClientWithTransport creates a new embedding client using a shared HTTP transport
// This allows multiple clients to share a single connection pool
func NewClientWithTransport(config common.EmbeddingConfig, inputChan chan common.TemplateResult, outputChan chan common.EmbeddingResult, transport *http.Transport) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		config: config,
		httpClient: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
		inputChan:      inputChan,
		outputChan:     outputChan,
		ctx:            ctx,
		cancel:         cancel,
		batch:          make([]batchItem, 0, config.BatchSize),
		batchTimerDone: make(chan struct{}),
	}
}

// Start begins processing template results
func (c *Client) Start() {
	go c.run()
}

// Stop stops the client
func (c *Client) Stop() {
	c.cancel()
}

func (c *Client) run() {
	c.batchTimer = time.NewTimer(c.config.BatchTimeout)
	defer c.batchTimer.Stop()

	for {
		select {
		case <-c.ctx.Done():
			// Flush any remaining batch
			if len(c.batch) > 0 {
				c.flushBatch()
			}
			close(c.outputChan)
			return

		case result, ok := <-c.inputChan:
			if !ok {
				// Flush remaining batch
				if len(c.batch) > 0 {
					c.flushBatch()
				}
				close(c.outputChan)
				return
			}

			// Add to batch
			c.batch = append(c.batch, batchItem{
				windowID:  result.WindowID,
				templates: result.Templates,
			})

			// Check if batch is full
			totalTemplates := 0
			for _, item := range c.batch {
				totalTemplates += len(item.templates)
			}

			if totalTemplates >= c.config.BatchSize {
				c.flushBatch()
				c.batchTimer.Reset(c.config.BatchTimeout)
			}

		case <-c.batchTimer.C:
			// Timeout: flush batch
			if len(c.batch) > 0 {
				c.flushBatch()
			}
			c.batchTimer.Reset(c.config.BatchTimeout)
		}
	}
}

func (c *Client) flushBatch() {
	if len(c.batch) == 0 {
		return
	}

	// Collect all templates
	allTemplates := make([]string, 0)
	templateToWindow := make(map[int]int)  // template index -> window ID
	windowTemplates := make(map[int][]int) // window ID -> template indices
	allWindows := make(map[int]bool)       // track all windows in batch

	idx := 0
	for _, item := range c.batch {
		allWindows[item.windowID] = true // Track this window
		for _, template := range item.templates {
			allTemplates = append(allTemplates, template)
			templateToWindow[idx] = item.windowID
			windowTemplates[item.windowID] = append(windowTemplates[item.windowID], idx)
			idx++
		}
	}

	// Get embeddings with retry (skip if no templates at all)
	var embeddings []common.Vector
	var err error
	if len(allTemplates) > 0 {
		embeddings, err = c.getEmbeddingsWithRetry(allTemplates)
		if err != nil {
			log.Errorf("Failed to get embeddings after retries: %v", err)
			c.batch = c.batch[:0] // Clear batch
			return
		}
	}

	// Send results for ALL windows (including those with no templates)
	// This ensures time-series continuity for DMD analysis
	for windowID := range allWindows {
		indices := windowTemplates[windowID]
		windowEmbeddings := make([]common.Vector, 0, len(indices))
		windowTemplateList := make([]string, 0, len(indices))

		for _, idx := range indices {
			if idx < len(embeddings) {
				windowEmbeddings = append(windowEmbeddings, embeddings[idx])
				windowTemplateList = append(windowTemplateList, allTemplates[idx])
			}
		}

		// Send result even if empty (important for time-series continuity)
		c.outputChan <- common.EmbeddingResult{
			WindowID:   windowID,
			Templates:  windowTemplateList,
			Embeddings: windowEmbeddings,
		}
	}

	// Clear batch
	c.batch = c.batch[:0]
}

func (c *Client) getEmbeddingsWithRetry(texts []string) ([]common.Vector, error) {
	var lastErr error

	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		embeddings, err := c.getEmbeddings(texts)
		if err == nil {
			return embeddings, nil
		}

		lastErr = err
		log.Warnf("Embedding request failed (attempt %d/%d): %v", attempt+1, c.config.MaxRetries, err)

		// Exponential backoff
		if attempt < c.config.MaxRetries-1 {
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			time.Sleep(backoff)
		}
	}

	return nil, fmt.Errorf("all retry attempts failed: %w", lastErr)
}

func (c *Client) getEmbeddings(texts []string) ([]common.Vector, error) {
	// Ollama expects array of strings
	req := embedRequest{
		Model: c.config.Model,
		Input: texts,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Use the configured ServerURL directly (should be http://localhost:11434/api/embed for Ollama)
	httpReq, err := http.NewRequestWithContext(c.ctx, "POST", c.config.ServerURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var embedResp embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Convert Ollama format to common.Vector
	if len(embedResp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(embedResp.Embeddings))
	}

	vectors := make([]common.Vector, len(embedResp.Embeddings))
	for i, emb := range embedResp.Embeddings {
		vectors[i] = common.Vector(emb)
	}

	return vectors, nil
}
