// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"fmt"
	"runtime"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

// PatternLogProcessor is a log processor that detects patterns in logs.
type PatternLogProcessor struct {
	ClustererPipeline *patterns.MultiThreadPipeline
	ResultChannel     chan *patterns.MultiThreadResult
}

func NewPatternLogProcessor() *PatternLogProcessor {
	resultChannel := make(chan *patterns.MultiThreadResult, 4096)
	clustererPipeline := patterns.NewMultiThreadPipeline(runtime.NumCPU(), resultChannel, false)

	p := &PatternLogProcessor{
		ClustererPipeline: clustererPipeline,
		ResultChannel:     resultChannel,
	}

	go func() {
		for result := range resultChannel {
			fmt.Printf("[cc] Processor: Result: %+v\n", result.ClusterResult.Signature)
		}
	}()

	return p
}

func (p *PatternLogProcessor) Name() string {
	return "pattern_log_processor"
}

func (p *PatternLogProcessor) Process(log observer.LogView) observer.LogProcessorResult {
	fmt.Printf("Processing log: %s\n", string(log.GetContent()))

	p.ClustererPipeline.Process(string(log.GetContent()))

	// Results arrive asynchronously via ResultChannel
	return observer.LogProcessorResult{}
}
