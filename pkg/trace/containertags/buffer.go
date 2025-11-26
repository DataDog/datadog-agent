// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containertagsbuffer contains the logic to buffer payloads for container tags
// enrichment
package containertagsbuffer

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-agent/pkg/trace/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

const (
	maxBufferDuration = 8 * time.Second
	// 5M is an order of magnitude less then trace-agent binary in memory
	maxSizeForNoLimit = int64(5_000_000)

	metricMemoryUsage      = "datadog.trace_agent.tag_buffer.memory_usage"
	metricPayloadsPending  = "datadog.trace_agent.tag_buffer.pending_payloads"
	metricPayloadsBuffered = "datadog.trace_agent.tag_buffer.buffered_payloads"
	metricDenied           = "datadog.trace_agent.tag_buffer.denied_payloads"
)

// ContainerTagsBuffer is a buffer for container tag resolution.
//
// In kubernetes, containers start and emit spans before container tags
// (pod, deployment..) are extracted from the kubelet.
// This buffer holds incoming tagging requests until specific tags (e.g., "kube_pod_name")
//
// Safety mechanisms
// - at most 10% of the max trace-agent memory can be used by payloads pending resolution
// - payloads can't be buffered more then `maxBufferDuration`
// - if we failed the resolution of a containerID before the time limit, further payloads
// from this containerID are not buffered
type ContainerTagsBuffer interface {
	Start()
	Stop()
	IsEnabled() bool
	AsyncEnrichment(containerID string, applyResult func([]string, error), payloadSize int64) (pending bool)
}

type containerTagsBuffer struct {
	conf        *config.AgentConfig
	resolveFunc func(string) ([]string, error)
	statsd      statsd.ClientInterface

	in        chan bufferInput
	exit      chan struct{}
	exitWG    sync.WaitGroup
	isRunning atomic.Bool

	deniedContainers *deniedContainers
	containersBuffer map[string]containerBuffer

	bufferDuration time.Duration
	hardTimeLimit  time.Duration
	maxSize        int64

	// memoryUsage tracks bytes of pending payloads
	memoryUsage atomic.Int64

	// shared stats
	hitHardTimeLimit      atomic.Int64
	hitMemoryLimit        atomic.Int64
	totalPayloadsPending  atomic.Int64
	totalPayloadsBuffered atomic.Int64
}

// NewContainerTagsBuffer creates a new buffer if it's enabled in configuration and the agent is in kubernetes
// elses returns a NoOpTagBuffer
func NewContainerTagsBuffer(conf *config.AgentConfig, statsd statsd.ClientInterface) ContainerTagsBuffer {
	if !conf.ContainerTagsBuffer {
		return &NoOpTagsBuffer{}
	}
	if !env.IsFeaturePresent(env.Kubernetes) {
		return &NoOpTagsBuffer{}
	}
	return newContainerTagsBuffer(conf, statsd)
}

func newContainerTagsBuffer(conf *config.AgentConfig, statsd statsd.ClientInterface) *containerTagsBuffer {
	ctb := &containerTagsBuffer{
		conf:             conf,
		statsd:           statsd,
		in:               make(chan bufferInput, 5),
		exit:             make(chan struct{}),
		deniedContainers: newDeniedContainers(),
		containersBuffer: make(map[string]containerBuffer, 100),

		bufferDuration: maxBufferDuration,
		hardTimeLimit:  maxBufferDuration + time.Second,
		maxSize:        int64(conf.MaxMemory * 0.1),
	}
	// set a reasonable max size when the agent is configured with no
	// memory boundary
	if ctb.maxSize == 0 {
		ctb.maxSize = maxSizeForNoLimit
	}
	ctb.resolveFunc = conf.ContainerTags
	return ctb
}

func (p *containerTagsBuffer) forceFlush() {
	for cid, buffer := range p.containersBuffer {
		ctags, _, err := p.resolveContainerTagsWithSource(cid)
		buffer.flush(tagResult{tags: ctags, err: err})
	}
}

func (p *containerTagsBuffer) resolvePendingContainers(now time.Time) {
	for cid, buffer := range p.containersBuffer {
		ctags, okSources, err := p.resolveContainerTagsWithSource(cid)
		// happy path, we resolved containers
		if okSources && err == nil {
			buffer.flush(tagResult{tags: ctags, err: nil})
			delete(p.containersBuffer, cid)
			continue
		}
		// wait longer
		if now.Before(buffer.expireTs) {
			continue
		}
		// force flush + deny
		buffer.flush(tagResult{tags: ctags, err: nil})
		delete(p.containersBuffer, cid)
		p.deniedContainers.deny(now, cid)
	}
}

// Stop flushes all pending payloads and stops the worker
func (p *containerTagsBuffer) Stop() {
	p.isRunning.Store(false)
	close(p.exit)
	p.exitWG.Wait()
}

// Start begins the background worker loop that
// 1. enqueues pre-validated buffer requests (memory usage is taken in account prior)
// 2. retries periodically tag resolution
// 3. flushes when max buffer duration is exceeded
func (p *containerTagsBuffer) Start() {
	p.isRunning.Store(true)
	p.exitWG.Add(1)
	go func() {
		log.Debug("Starting container tags buffer with memory limit: ", p.maxSize)
		defer p.exitWG.Done()
		resolveTicker := time.NewTicker(1 * time.Second)
		defer resolveTicker.Stop()
		statsTicker := time.NewTicker(10 * time.Second)
		defer statsTicker.Stop()
		for {
			select {
			case <-p.exit:
				p.forceFlush()
				return
			case toBuffer := <-p.in:
				p.buffer(toBuffer)
			case now := <-resolveTicker.C:
				p.resolvePendingContainers(now)
			case <-statsTicker.C:
				p.report()
				p.deniedContainers.report(p.statsd)
			}
		}
	}()
}

func (p *containerTagsBuffer) report() {
	if hardTimeLimit := p.hitHardTimeLimit.Swap(0); hardTimeLimit > 0 {
		_ = p.statsd.Count(metricDenied, hardTimeLimit, []string{"reason:hardtimelimit"}, 1)
	}
	if memoryLimit := p.hitMemoryLimit.Swap(0); memoryLimit > 0 {
		_ = p.statsd.Count(metricDenied, memoryLimit, []string{"reason:memorylimit"}, 1)
	}
	if payloadsBuffered := p.totalPayloadsBuffered.Swap(0); payloadsBuffered > 0 {
		_ = p.statsd.Count(metricPayloadsBuffered, payloadsBuffered, nil, 1)
	}
	payloadsPending := p.totalPayloadsPending.Load()
	_ = p.statsd.Gauge(metricPayloadsPending, float64(payloadsPending), nil, 1)
	memoryUsage := p.memoryUsage.Load()
	_ = p.statsd.Gauge(metricMemoryUsage, float64(memoryUsage), nil, 1)
}

func (p *containerTagsBuffer) buffer(in bufferInput) {
	cb, ok := p.containersBuffer[in.cid]
	if !ok {
		cb = containerBuffer{
			expireTs:      in.now.Add(p.bufferDuration),
			pendingResult: make([]func(tagResult), 0, 1),
		}

	}
	cb.pendingResult = append(cb.pendingResult, in.onResult)
	p.containersBuffer[in.cid] = cb
}

// IsEnabled is true if the buffer has been started
func (p *containerTagsBuffer) IsEnabled() bool {
	return p.isRunning.Load()
}

func (p *containerTagsBuffer) resolveContainerTagsWithSource(containerID string) ([]string, bool, error) {
	ctags, err := p.resolveFunc(containerID)
	// cheat - testing kube tag presence, waiting for tagger to expose source
	var okSource bool
	for _, tag := range ctags {
		if !strings.HasPrefix(tag, "kube_") {
			continue
		}
		okSource = true
		break
	}
	return ctags, okSource, err
}

func (p *containerTagsBuffer) registerMemory(payloadSize int64) (bool, func()) {
	releaseMemory := func() {
		p.memoryUsage.Add(-payloadSize)
	}
	if p.memoryUsage.Add(payloadSize) > int64(p.maxSize) {
		p.hitMemoryLimit.Add(1)
		return false, releaseMemory
	}
	return true, releaseMemory
}

// AsyncEnrichment attempts to resolve tags for a specific container ID.
//
// Parameters:
//   - containerID: target container to resolve tags for.
//   - applyResult: a callback function executed when tags are found, the buffer times out, or the buffer is bypassed.
//   - payloadSize: size in bytes of the data associated with this request, used for memory pressure limits
//
// Returns:
//   - true (Pending): The container is missing critical tags (e.g., "kube_") and resolution is buffered
//   - false (Resolved/Skipped): The tags are ready, the buffer is full, or the container is denied.
//     The 'applyResult' callback has already been called synchronously.
func (p *containerTagsBuffer) AsyncEnrichment(containerID string, applyResult func([]string, error), payloadSize int64) (pending bool) {
	ctags, okSources, err := p.resolveContainerTagsWithSource(containerID)
	// happy path complete container tags
	if okSources && err == nil {
		applyResult(ctags, err)
		return false
	}
	if !p.IsEnabled() {
		applyResult(ctags, err)
		return false
	}
	now := time.Now()
	if p.deniedContainers.shouldDeny(now, containerID) {
		applyResult(ctags, err)
		return false
	}

	enoughMemory, releasePayloadSize := p.registerMemory(payloadSize)
	if !enoughMemory {
		applyResult(ctags, err)
		releasePayloadSize()
		return false
	}
	p.totalPayloadsPending.Add(1)
	p.totalPayloadsBuffered.Add(1)

	resChan := make(chan tagResult, 1)
	go func() {
		defer releasePayloadSize()
		defer p.totalPayloadsPending.Add(-1)
		select {
		case p.in <- bufferInput{
			cid:      containerID,
			now:      now,
			onResult: func(tr tagResult) { resChan <- tr },
		}:
		case <-p.exit:
			applyResult(ctags, err)
			return
		}

		select {
		case res := <-resChan:
			applyResult(res.tags, res.err)
			return
		case <-p.exit:
			applyResult(ctags, err)
			return
		case timeout := <-time.After(p.hardTimeLimit):
			p.deniedContainers.deny(timeout, containerID)
			applyResult(ctags, err)
			p.hitHardTimeLimit.Add(1)
			return
		}
	}()
	return true
}

type tagResult struct {
	tags []string
	err  error
}

type containerBuffer struct {
	pendingResult []func(tagResult)
	expireTs      time.Time
}

func (b *containerBuffer) flush(res tagResult) {
	for _, fn := range b.pendingResult {
		fn(res)
	}
}

type bufferInput struct {
	onResult func(tagResult)
	cid      string
	now      time.Time
}
