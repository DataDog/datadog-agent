// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"encoding/binary"
	"hash"
	"hash/fnv"
	"runtime"
	"strings"
	"sync/atomic"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/comp/observer/impl/patterns"
)

var tlmAnomalyCount = atomic.Int64{}

// PatternLogProcessor is a log processor that detects patterns in logs.
// Pattern clusterers are distinct based on the tags logs have (https://datadoghq.atlassian.net/wiki/x/CQB6Ng):
// - env
// - service
// - source
// - pod_name
// - dirname
type PatternLogProcessor struct {
	ClustererPipeline *patterns.MultiThreadPipeline[*LogADTags]
	ResultChannel     chan *patterns.MultiThreadResult[*LogADTags]
	AnomalyDetectors  []LogAnomalyDetector
}

func NewPatternLogProcessor(anomalyDetectors []LogAnomalyDetector) *PatternLogProcessor {
	pipelineOutputChannel := make(chan *patterns.MultiThreadResult[*LogADTags], 4096)
	clustererPipeline := patterns.NewMultiThreadPipeline(runtime.NumCPU(), pipelineOutputChannel, false)

	p := &PatternLogProcessor{
		ClustererPipeline: clustererPipeline,
		ResultChannel:     pipelineOutputChannel,
		AnomalyDetectors:  anomalyDetectors,
	}

	for _, anomalyDetector := range p.AnomalyDetectors {
		anomalyDetector.SetProcessor(p)
		anomalyDetector.Run()
	}

	go func() {
		for result := range pipelineOutputChannel {
			for _, anomalyDetector := range p.AnomalyDetectors {
				anomalyDetector.Process(result.ClusterInput, result.ClusterResult)
			}
		}
	}()

	return p
}

func (p *PatternLogProcessor) Name() string {
	return "pattern_log_processor"
}

func (p *PatternLogProcessor) Process(log observer.LogView) observer.LogProcessorResult {
	tags := ParseTags(log.GetTags())
	p.ClustererPipeline.Process(&patterns.TokenizerInput[*LogADTags]{Message: string(log.GetContent()), UserData: &tags})

	// Results arrive asynchronously via ResultChannel
	return observer.LogProcessorResult{}
}

// What we plug after the log processor
type LogAnomalyDetector interface {
	SetProcessor(processor *PatternLogProcessor)
	// May start the go routine to process logs / patterns asynchronously
	Run()
	Name() string
	// Called for each input from the clusterer pipeline (can be added in a batched and processed asynchronously)
	Process(clustererInput *patterns.ClustererInput[*LogADTags], clustererResult *patterns.ClusterResult)
}

type LogAnomalyDetectionProcessInput struct {
	ClustererInput  *patterns.ClustererInput[*LogADTags]
	ClustererResult *patterns.ClusterResult
}

// We group anomalies by cluster ID and some tags
type LogADGroupByKey struct {
	ClusterID    int
	Tags         *LogADTags
	computedHash int64
}

func (key *LogADGroupByKey) Hash() int64 {
	if key.computedHash != 0 {
		return key.computedHash
	}

	hash := fnv.New64()
	binary.Write(hash, binary.LittleEndian, int32(key.ClusterID))
	hash.Write([]byte{0})
	key.Tags.WriteHash(hash)

	key.computedHash = int64(hash.Sum64())

	return key.computedHash
}

// Tags that are used to group anomalies
// Tags could be empty
// TODO(celian): Verify that we can get these tags from the observer (not present when run locally)
type LogADTags struct {
	// We use a fixed length array to avoid allocations
	TagValues [len(logADAllTags)]string
}

// All the tags used to group anomalies
var logADAllTags = [...]string{
	"dirname",
	"env",
	"filename",
	"pod_name",
	"service",
	"source",
}

func ParseTags(tags []string) LogADTags {
	result := LogADTags{}
	for _, tag := range tags {
		for i, tagName := range logADAllTags {
			if strings.HasPrefix(tag, tagName+":") {
				result.TagValues[i] = strings.TrimPrefix(tag, tagName+":")
				break
			}
		}
	}

	return result
}

func (tags *LogADTags) FullTags() []string {
	res := make([]string, 0, len(logADAllTags))

	for i, tagName := range logADAllTags {
		if tags.TagValues[i] != "" {
			res = append(res, tagName+":"+tags.TagValues[i])
		}
	}

	return res
}

func (tags *LogADTags) WriteHash(hash hash.Hash64) {
	for i := range len(logADAllTags) {
		if tags.TagValues[i] != "" {
			hash.Write([]byte(tags.TagValues[i]))
		}
		// Separator even if empty
		hash.Write([]byte{0})
	}
}
