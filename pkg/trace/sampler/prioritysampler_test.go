// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"math/rand"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/cihub/seelog"
	"github.com/stretchr/testify/assert"
)

const (
	testServiceA = "service-a"
	testServiceB = "service-b"
)

func randomTraceID() uint64 {
	return uint64(rand.Int63())
}

func getTestPrioritySampler() *PrioritySampler {
	// Disable debug logs in these tests
	seelog.UseLogger(seelog.Disabled)

	// No extra fixed sampling, no maximum TPS
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       0.0,
	}

	return NewPrioritySampler(conf, &DynamicConfig{})
}

func getTestTraceWithService(t *testing.T, service string, s *PrioritySampler) (*pb.TraceChunk, *pb.Span) {
	tID := randomTraceID()
	spans := []*pb.Span{
		{TraceID: tID, SpanID: 1, ParentID: 0, Start: 42, Duration: 1000000, Service: service, Type: "web", Meta: map[string]string{"env": defaultEnv}, Metrics: map[string]float64{}},
		{TraceID: tID, SpanID: 2, ParentID: 1, Start: 100, Duration: 200000, Service: service, Type: "sql"},
	}
	r := rand.Float64()
	priority := PriorityAutoDrop
	rates := s.ratesByService()
	key := ServiceSignature{spans[0].Service, defaultEnv}
	var rate float64
	if serviceRate, ok := rates[key]; ok {
		rate = serviceRate.r
		spans[0].Metrics[agentRateKey] = serviceRate.r
	} else {
		rate = 1
	}
	if r <= rate {
		priority = PriorityAutoKeep
	}
	return &pb.TraceChunk{
		Priority: int32(priority),
		Spans:    spans,
	}, spans[0]
}

func TestPrioritySample(t *testing.T) {
	// Simple sample unit test
	assert := assert.New(t)

	env := defaultEnv

	s := getTestPrioritySampler()

	assert.Equal(int64(0), s.localRates.totalSeen, "checking fresh backend total score is 0")
	assert.Equal(int64(0), s.localRates.totalKept, "checkeing fresh backend sampled score is 0")

	s = getTestPrioritySampler()
	chunk, root := getTestTraceWithService(t, "my-service", s)

	chunk.Priority = -1
	sampled := s.Sample(time.Now(), chunk, root, env, false)
	assert.False(sampled, "trace with negative priority is dropped")
	assert.Equal(int64(0), s.localRates.totalSeen, "sampling a priority -1 trace should *NOT* impact sampler backend")
	assert.Equal(int64(0), s.localRates.totalKept, "sampling a priority -1 trace should *NOT* impact sampler backend")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 0
	sampled = s.Sample(time.Now(), chunk, root, env, false)
	assert.False(sampled, "trace with priority 0 is dropped")
	assert.True(int64(0) < s.localRates.totalSeen, "sampling a priority 0 trace should increase total score")
	assert.Equal(int64(0), s.localRates.totalKept, "sampling a priority 0 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 1
	sampled = s.Sample(time.Now(), chunk, root, env, false)
	assert.True(sampled, "trace with priority 1 is kept")
	assert.True(int64(0) < s.localRates.totalSeen, "sampling a priority 0 trace should increase total score")
	assert.True(int64(0) < s.localRates.totalKept, "sampling a priority 0 trace should increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = 2
	sampled = s.Sample(time.Now(), chunk, root, env, false)
	assert.True(sampled, "trace with priority 2 is kept")
	assert.Equal(int64(0), s.localRates.totalSeen, "sampling a priority 2 trace should *NOT* increase total score")
	assert.Equal(int64(0), s.localRates.totalKept, "sampling a priority 2 trace should *NOT* increase sampled score")

	s = getTestPrioritySampler()
	chunk, root = getTestTraceWithService(t, "my-service", s)

	chunk.Priority = int32(PriorityUserKeep)
	sampled = s.Sample(time.Now(), chunk, root, env, false)
	assert.True(sampled, "trace with high priority is kept")
	assert.Equal(int64(0), s.localRates.totalSeen, "sampling a high priority trace should *NOT* increase total score")
	assert.Equal(int64(0), s.localRates.totalKept, "sampling a high priority trace should *NOT* increase sampled score")

	chunk.Priority = int32(PriorityNone)
	sampled = s.Sample(time.Now(), chunk, root, env, false)
	assert.False(sampled, "this should not happen but a trace without priority sampling set should be dropped")
}

func TestPrioritySamplerWithNilRemote(t *testing.T) {
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       0.0,
	}
	s := NewPrioritySampler(conf, NewDynamicConfig())
	s.Start()
	s.updateRates()
	s.reportStats()
	chunk, root := getTestTraceWithService(t, "my-service", s)
	assert.True(t, s.Sample(time.Now(), chunk, root, "", false))
	s.Stop()
}

func TestPrioritySamplerWithRemote(t *testing.T) {
	conf := &config.AgentConfig{
		ExtraSampleRate: 1.0,
		TargetTPS:       0.0,
	}
	s := NewPrioritySampler(conf, NewDynamicConfig())
	s.remoteRates = newRemoteRates(10)
	s.Start()
	s.updateRates()
	s.reportStats()
	chunk, root := getTestTraceWithService(t, "my-service", s)
	assert.True(t, s.Sample(time.Now(), chunk, root, "", false))
	s.Stop()
}
