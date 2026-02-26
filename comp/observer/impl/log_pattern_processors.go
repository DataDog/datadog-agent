// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// PatternLogProcessor is a log processor that clusterizes logs into patterns.
// Patterns will be sent to various pattern based anomaly detectors.

import (
	"encoding/binary"
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
	hash.Write([]byte(key.Tags.Env))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.PodName))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.Service))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.Source))
	hash.Write([]byte{0})
	hash.Write([]byte(key.Tags.DirName))

	key.computedHash = int64(hash.Sum64())

	return key.computedHash
}

// Tags that are used to group anomalies
// Tags could be empty
type LogADTags struct {
	// /!\ Don't forget to update the Hash method when adding a new tag
	// TODO(celian): Verify that we can get these tags from the observer (not present when run locally)
	Env     string
	PodName string
	Service string
	Source  string
	// TODO(celian): Should we prefer dirname over filepath? Use both?
	DirName string
}

func ParseTags(tags []string) LogADTags {
	result := LogADTags{}
	for _, tag := range tags {
		if strings.HasPrefix(tag, "env:") {
			result.Env = strings.TrimPrefix(tag, "env:")
		} else if strings.HasPrefix(tag, "pod_name:") {
			result.PodName = strings.TrimPrefix(tag, "pod_name:")
		} else if strings.HasPrefix(tag, "service:") {
			result.Service = strings.TrimPrefix(tag, "service:")
		} else if strings.HasPrefix(tag, "source:") {
			result.Source = strings.TrimPrefix(tag, "source:")
		} else if strings.HasPrefix(tag, "dirname:") {
			result.DirName = strings.TrimPrefix(tag, "dirname:")
		}
	}

	return result
}

func (tags *LogADTags) FullTags() []string {
	res := make([]string, 0, 10)

	if tags.Env != "" {
		res = append(res, "env:"+tags.Env)
	}
	if tags.PodName != "" {
		res = append(res, "pod_name:"+tags.PodName)
	}
	if tags.Service != "" {
		res = append(res, "service:"+tags.Service)
	}
	if tags.Source != "" {
		res = append(res, "source:"+tags.Source)
	}
	if tags.DirName != "" {
		res = append(res, "dirname:"+tags.DirName)
	}

	return res
}
