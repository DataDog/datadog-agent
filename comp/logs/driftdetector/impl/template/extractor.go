// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package template

import (
	"context"
	"math"
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/comp/logs/driftdetector/impl/common"
)

// Extractor performs Shannon entropy-based template extraction
type Extractor struct {
	config     common.TemplateConfig
	inputChan  chan common.Window
	outputChan chan common.TemplateResult
	ctx        context.Context
	cancel     context.CancelFunc
	workerWg   sync.WaitGroup
}

// NewExtractor creates a new template extractor
func NewExtractor(config common.TemplateConfig, inputChan chan common.Window, outputChan chan common.TemplateResult) *Extractor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Extractor{
		config:     config,
		inputChan:  inputChan,
		outputChan: outputChan,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins processing windows with multiple workers
func (e *Extractor) Start() {
	for i := 0; i < e.config.WorkerCount; i++ {
		e.workerWg.Add(1)
		go e.worker()
	}

	// Goroutine to close output channel when all workers are done
	go func() {
		e.workerWg.Wait()
		close(e.outputChan)
	}()
}

// Stop stops the extractor
func (e *Extractor) Stop() {
	e.cancel()
}

func (e *Extractor) worker() {
	defer e.workerWg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return

		case window, ok := <-e.inputChan:
			if !ok {
				return
			}
			result := e.extractTemplates(window)
			e.outputChan <- result
		}
	}
}

func (e *Extractor) extractTemplates(window common.Window) common.TemplateResult {
	if len(window.Logs) == 0 {
		return common.TemplateResult{
			WindowID:         window.ID,
			Templates:        []string{},
			CompressionRatio: 1.0,
		}
	}

	// Step 1: Tokenize all logs
	tokenizedLogs := make([][]string, len(window.Logs))
	for i, log := range window.Logs {
		tokenizedLogs[i] = strings.Fields(log.Content)
	}

	// Step 2: Group by token count (bucketing)
	buckets := make(map[int][][]string)
	for _, tokens := range tokenizedLogs {
		length := len(tokens)
		buckets[length] = append(buckets[length], tokens)
	}

	// Step 3: Extract templates from each bucket
	templates := make(map[string]struct{})
	for _, bucket := range buckets {
		if len(bucket) == 0 {
			continue
		}

		template := e.extractTemplateFromBucket(bucket)
		if len(template) <= e.config.MaxCharacters {
			templates[template] = struct{}{}
		}
	}

	// Convert to slice
	templateSlice := make([]string, 0, len(templates))
	for t := range templates {
		templateSlice = append(templateSlice, t)
	}

	// Calculate compression ratio
	compressionRatio := float64(len(window.Logs)) / math.Max(float64(len(templateSlice)), 1.0)

	return common.TemplateResult{
		WindowID:         window.ID,
		Templates:        templateSlice,
		CompressionRatio: compressionRatio,
	}
}

func (e *Extractor) extractTemplateFromBucket(logs [][]string) string {
	if len(logs) == 0 {
		return ""
	}

	// All logs in bucket have same token count
	tokenCount := len(logs[0])
	if tokenCount == 0 {
		return ""
	}

	// For each token position, decide if it's constant, enum, or variable
	templateTokens := make([]string, tokenCount)

	for pos := 0; pos < tokenCount; pos++ {
		// Collect all tokens at this position
		positionTokens := make([]string, len(logs))
		for i, log := range logs {
			if pos < len(log) {
				positionTokens[i] = log[pos]
			}
		}

		// Calculate entropy and cardinality
		entropy := calculateEntropy(positionTokens)
		cardinality := countUnique(positionTokens)

		// Decide token type
		if entropy <= e.config.EntropyThreshold {
			// Low entropy: constant field
			templateTokens[pos] = positionTokens[0]
		} else if cardinality < e.config.EnumCardinalityThreshold {
			// High entropy but low cardinality: enum field
			templateTokens[pos] = positionTokens[0]
		} else {
			// High entropy and high cardinality: variable field
			templateTokens[pos] = "<*>"
		}
	}

	return strings.Join(templateTokens, " ")
}

// calculateEntropy computes Shannon entropy for a list of tokens
func calculateEntropy(tokens []string) float64 {
	if len(tokens) == 0 {
		return 0.0
	}

	// Count occurrences
	counts := make(map[string]int)
	for _, token := range tokens {
		counts[token]++
	}

	// Calculate entropy
	total := float64(len(tokens))
	entropy := 0.0

	for _, count := range counts {
		if count > 0 {
			p := float64(count) / total
			entropy -= p * math.Log2(p)
		}
	}

	return entropy
}

// countUnique returns the number of unique values in a slice
func countUnique(tokens []string) int {
	unique := make(map[string]struct{})
	for _, token := range tokens {
		unique[token] = struct{}{}
	}
	return len(unique)
}
