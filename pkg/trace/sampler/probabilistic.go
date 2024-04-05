// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package sampler

import (
	"encoding/binary"
	"encoding/hex"
	"hash/fnv"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	// These constants exist to match the behavior of the OTEL probabilistic sampler.
	numProbabilisticBuckets = 0x4000
	bitMaskHashBuckets      = numProbabilisticBuckets - 1
	percentageScaleFactor   = numProbabilisticBuckets / 100.0
)

// ProbabilisticSampler is a sampler that overrides all other samplers,
// it deterministically samples incoming traces by a hash of their trace ID
type ProbabilisticSampler struct {
	enabled                  bool
	hashSeed                 []byte
	scaledSamplingPercentage uint32
}

// NewProbabilisticSampler returns a new ProbabilisticSampler that deterministically samples
// a given percentage of incoming spans based on their trace ID
func NewProbabilisticSampler(conf *config.AgentConfig) *ProbabilisticSampler {
	hashSeedBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(hashSeedBytes, conf.ProbabilisticSamplerHashSeed)
	ps := &ProbabilisticSampler{
		enabled:                  conf.ProbabilisticSamplerEnabled,
		hashSeed:                 hashSeedBytes,
		scaledSamplingPercentage: uint32(conf.ProbabilisticSamplerSamplingPercentage * percentageScaleFactor),
	}
	return ps
}

// Sample a trace given the chunk's root span, returns true if the trace should be kept
func (ps *ProbabilisticSampler) Sample(root *trace.Span) bool {
	if !ps.enabled {
		return false
	}

	tid, err := get128BitTraceID(root)
	if err != nil {
		log.Errorf("Unable to probabilistically sample, failed to determine 128-bit trace ID from incoming span: %v", err)
		return false
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write(ps.hashSeed)
	_, _ = hasher.Write(tid)
	hash := hasher.Sum32()
	return hash&bitMaskHashBuckets < ps.scaledSamplingPercentage
}

func get128BitTraceID(span *trace.Span) ([]byte, error) {
	// If it's an otel span the whole trace ID is in otel.trace
	if tid, ok := span.Meta["otel.trace_id"]; ok {
		bs, err := hex.DecodeString(tid)
		if err != nil {
			return nil, err
		}
		return bs, nil
	}
	tid := make([]byte, 16)
	binary.BigEndian.PutUint64(tid[8:], span.TraceID)
	// Get hex encoded upper bits for datadog spans
	// If no value is found we can use the default `0` value as that's what will have been propagated
	if upper, ok := span.Meta["_dd.p.tid"]; ok {
		u, err := strconv.ParseUint(upper, 16, 64)
		if err != nil {
			return nil, err
		}
		binary.BigEndian.PutUint64(tid[:8], u)
	}
	return tid, nil
}
