// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containertagsbuffer contains the logic to buffer payloads for container tags
// enrichment
package containertagsbuffer

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResolver struct {
	mu   sync.RWMutex
	tags []string
	err  error
}

func (m *mockResolver) setTags(t []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tags = t
}

func (m *mockResolver) Resolve(string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tags, m.err
}

func TestBuffer_DelayedSuccess(t *testing.T) {
	mock := &mockResolver{tags: []string{"short_image:java"}}

	buff := newContainerTagsBuffer(&config.AgentConfig{
		ContainerTagsBuffer: true,
		MaxMemory:           2000.0,
	}, &statsd.NoOpClient{})
	buff.resolveFunc = mock.Resolve
	buff.Start()
	defer close(buff.exit)

	resultCh := make(chan []string, 1)

	var calledOnce atomic.Int64
	onResolution := func(ctags []string, _ error) {
		calledOnce.Add(1)
		resultCh <- ctags
	}
	pending := buff.AsyncEnrichment("c-delayed", onResolution, 100)

	assert.True(t, pending)

	select {
	case <-resultCh:
		t.Fatal("Should have blocked waiting for kube tags")
	case <-time.After(200 * time.Millisecond):
		// it's blocked
	}

	mock.setTags([]string{"short_image:nginx", "kube_pod_name:app"})

	select {
	case tags := <-resultCh:
		assert.Contains(t, tags, "kube_pod_name:app", "Should have received the new tags")
	case <-time.After(10 * time.Second):
		t.Fatal("Timed out waiting for buffer to resolve")
	}
	assert.Equal(t, calledOnce.Load(), int64(1))
}

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		running  bool
		expected bool
	}{
		{"Running", true, true},
		{"Not running", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := &config.AgentConfig{}
			ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})

			if tt.running {
				ctb.Start()
			}

			assert.Equal(t, tt.expected, ctb.IsEnabled())
			ctb.Stop()
		})
	}
}

func TestMaxSizeNoLimit(t *testing.T) {
	conf := &config.AgentConfig{}
	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	assert.Equal(t, ctb.maxSize, maxSizeForNoLimit)
}

func TestAsyncEnrichment_DeniedContainer(t *testing.T) {
	conf := &config.AgentConfig{
		MaxMemory:           1000,
		ContainerTagsBuffer: true,
		ContainerTags: func(string) ([]string, error) {
			return []string{"image:only"}, nil
		},
	}

	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.Start()
	defer ctb.Stop()

	cid := "bad-container"

	// Pre-deny the container
	ctb.deniedContainers.deny(time.Now(), cid)

	called := false
	cb := func([]string, error) { called = true }

	pending := ctb.AsyncEnrichment(cid, cb, 50)

	assert.False(t, pending, "Denied container should not be pending")
	assert.True(t, called, "Callback should be called immediately")
	assert.Equal(t, int64(0), ctb.memoryUsage.Load(), "Should not consume buffer memory")
}

func TestAsyncEnrichment_MemoryLimit(t *testing.T) {
	mock := &mockResolver{tags: []string{"short_image:java"}}
	conf := &config.AgentConfig{
		MaxMemory:           100, // Max size will be 10 (10%)
		ContainerTagsBuffer: true,
		ContainerTags:       mock.Resolve,
	}

	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.Start()
	defer ctb.Stop()

	// 1. Fill memory (Payload 10 fills the 10 limit)
	ctb.AsyncEnrichment("container-1", func([]string, error) {}, 10)

	// Wait for it to hit the map
	require.Eventually(t, func() bool {
		return ctb.memoryUsage.Load() == 10
	}, 1*time.Second, 10*time.Millisecond)

	// 2. Try to add another container
	called := false
	cb := func([]string, error) { called = true }

	pending := ctb.AsyncEnrichment("container-2", cb, 1)

	assert.False(t, pending, "Should be rejected due to memory limit")
	assert.True(t, called, "Callback should be called immediately on rejection")
	assert.Equal(t, int64(10), ctb.memoryUsage.Load())

	// 3. memory cleaned post resolution
	mock.setTags([]string{"kube_t:a"})
	require.Eventually(t, func() bool {
		return ctb.memoryUsage.Load() == 0
	}, 3*time.Second, 10*time.Millisecond)
}

func TestAsyncEnrichment_ImmediateResolution(t *testing.T) {
	conf := &config.AgentConfig{
		MaxMemory:           10000,
		ContainerTagsBuffer: true,
		ContainerTags: func(string) ([]string, error) {
			return []string{"kube_pod_name:abc", "image:123"}, nil
		},
	}
	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.Start()
	defer ctb.Stop()

	var wg sync.WaitGroup
	wg.Add(1)

	callback := func(tags []string, err error) {
		assert.Contains(t, tags, "kube_pod_name:abc")
		assert.NoError(t, err)
		wg.Done()
	}

	pending := ctb.AsyncEnrichment("container-1", callback, 100)
	assert.False(t, pending, "Should return false as tags were resolved immediately")
	wg.Wait()
}

func TestAsyncEnrichment_Buffered_Expiration(t *testing.T) {
	conf := &config.AgentConfig{
		MaxMemory:           10000,
		ContainerTagsBuffer: true,
		ContainerTags: func(string) ([]string, error) {
			return []string{"image:only"}, nil
		},
	}

	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.bufferDuration = 1 * time.Nanosecond
	ctb.Start()

	resultChan := make(chan []string, 1)
	callback := func(tags []string, _ error) {
		resultChan <- tags
	}

	pending := ctb.AsyncEnrichment("container-expire", callback, 100)
	assert.True(t, pending)

	assert.Equal(t, ctb.memoryUsage.Load(), int64(100))
	select {
	case tags := <-resultChan:
		assert.Contains(t, tags, "image:only")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for expiration flush")
	}
	// Memory is released via defer after the callback returns, so use Eventually
	// to avoid a race between receiving the callback result and the defer executing
	require.Eventually(t, func() bool {
		return ctb.memoryUsage.Load() == 0
	}, 1*time.Second, 10*time.Millisecond, "memory should be released after callback")

	// container is now denied
	assert.True(t, ctb.deniedContainers.shouldDeny(time.Now(), "container-expire"))
}

func TestAsyncEnrichment_Buffered_HardLimit(t *testing.T) {
	conf := &config.AgentConfig{
		MaxMemory:           10000,
		ContainerTagsBuffer: true,
		ContainerTags: func(string) ([]string, error) {
			return []string{"image:only"}, nil
		},
	}

	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.hardTimeLimit = 100 * time.Millisecond
	ctb.Start()

	resultChan := make(chan []string, 1)
	callback := func(tags []string, _ error) {
		resultChan <- tags
	}

	pending := ctb.AsyncEnrichment("container-expire", callback, 100)
	assert.True(t, pending)

	assert.Equal(t, ctb.memoryUsage.Load(), int64(100))
	select {
	case tags := <-resultChan:
		assert.Contains(t, tags, "image:only")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for expiration flush")
	}
	// Memory is released via defer after the callback returns, so use Eventually
	// to avoid a race between receiving the callback result and the defer executing
	require.Eventually(t, func() bool {
		return ctb.memoryUsage.Load() == 0
	}, 1*time.Second, 10*time.Millisecond, "memory should be released after callback")

	// container is now denied
	assert.True(t, ctb.deniedContainers.shouldDeny(time.Now(), "container-expire"))
}

func TestAsyncEnrichment_Concurrent_MixedScenarios(t *testing.T) {
	synctest.Test(t, syncTestAsyncEnrichmentConcurrentMixedScenarios)
}

func syncTestAsyncEnrichmentConcurrentMixedScenarios(t *testing.T) {
	containerIDs := []string{"c-error", "c-to-resolve-1", "c-to-resolve-2", "c-will-expire1", "c-will-expire2"}

	var shouldResolveContainers atomic.Bool

	conf := &config.AgentConfig{
		MaxMemory:           10 * 1024 * 1024,
		ContainerTagsBuffer: true,
		ContainerTags: func(cid string) ([]string, error) {
			if strings.Contains(cid, "c-error") {
				return nil, errors.New("container not found")
			}

			if shouldResolveContainers.Load() {
				return []string{"kube_image:" + cid}, nil
			}
			return []string{"tag:incomplete_tags"}, nil
		},
	}

	ctb := newContainerTagsBuffer(conf, &statsd.NoOpClient{})
	ctb.Start()

	var wg sync.WaitGroup
	var totalExecuted atomic.Int64
	var totalAsyncCalls atomic.Int64

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				cid := containerIDs[rand.Intn(len(containerIDs))]

				totalAsyncCalls.Add(1)
				ctb.AsyncEnrichment(cid, func([]string, error) { totalExecuted.Add(1) }, 100)
			}
		}()
	}

	time.Sleep(maxBufferDuration + time.Second) // advance time to allow ticker fires and container expiration
	shouldResolveContainers.Store(true)
	t.Log("--- SWITCHING STATE: Containers should now resolve with kube_ tags ---")
	time.Sleep(2 * time.Second) // advance time to allow resolution of buffered containers

	wg.Wait()
	ctb.Stop()
	synctest.Wait()

	// memory usage release happens in a deferred call inside the AsyncEnrichment goroutines.
	assert.Zero(t, ctb.memoryUsage.Load(), "Memory usage should be 0")

	assert.True(t, ctb.deniedContainers.shouldDeny(time.Now(), "c-error"))
	assert.True(t, ctb.deniedContainers.shouldDeny(time.Now(), "c-will-expire1"))
	assert.True(t, ctb.deniedContainers.shouldDeny(time.Now(), "c-will-expire2"))
	assert.Equal(t, totalExecuted.Load(), totalAsyncCalls.Load())
}
