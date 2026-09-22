// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containerd/fake"
)

type hiddenBytesTestStore struct {
	workloadmeta.Component
	mu     sync.Mutex
	events []workloadmeta.CollectorEvent
}

func (s *hiddenBytesTestStore) Notify(events []workloadmeta.CollectorEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
}

func newHiddenBytesTestCollector(scan hiddenBytesScan) (*collector, *hiddenBytesTestStore) {
	store := &hiddenBytesTestStore{}
	return &collector{
		store: store,
		hiddenBytes: &hiddenBytesCollector{
			wake:  make(chan struct{}, 1),
			done:  make(chan struct{}),
			scan:  scan,
			jobs:  make(map[string]*hiddenBytesJob),
			cache: newHiddenBytesCache(""),
		},
	}, store
}

func hiddenBytesTestImage(name string) *workloadmeta.ContainerImageMetadata {
	return &workloadmeta.ContainerImageMetadata{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: digest.FromString(name).String()},
		EntityMeta: workloadmeta.EntityMeta{Name: name, Namespace: "k8s.io"},
		Layers: []workloadmeta.ContainerImageLayer{
			{DiffID: digest.FromString("base").String()},
			{History: &ocispec.History{EmptyLayer: true}},
			{DiffID: digest.FromString("top").String()},
		},
	}
}

func hiddenBytesTestResult(img *workloadmeta.ContainerImageMetadata) []hiddenLayerResult {
	return []hiddenLayerResult{
		{DiffID: img.Layers[0].DiffID, Bytes: 10},
		{DiffID: img.Layers[2].DiffID, Bytes: 0},
	}
}

func publishHiddenBytesTestImage(c *collector, img *workloadmeta.ContainerImageMetadata) {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()
	c.publishImageLocked(img)
}

func TestHiddenBytesCoalescingAndSaturatedWake(t *testing.T) {
	scans := 0
	c, store := newHiddenBytesTestCollector(func(_ context.Context, img *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		scans++
		return hiddenBytesTestResult(img), nil
	})
	for i := 0; i < 4; i++ {
		img := hiddenBytesTestImage(fmt.Sprintf("image-%d", i))
		publishHiddenBytesTestImage(c, img)
		publishHiddenBytesTestImage(c, img)
	}
	require.Len(t, c.hiddenBytes.wake, 1)
	require.Len(t, c.hiddenBytes.jobs, 4)
	for i := 0; i < 4; i++ {
		img, job := c.nextHiddenBytesImage(time.Now())
		require.NotNil(t, img)
		c.collectHiddenBytes(t.Context(), img, job)
		updated := c.latestImages[img.ID]
		require.NotNil(t, updated.Layers[0].HiddenBytes)
		assert.Equal(t, uint64(10), *updated.Layers[0].HiddenBytes)
		assert.Nil(t, updated.Layers[1].HiddenBytes)
		require.NotNil(t, updated.Layers[2].HiddenBytes)
		assert.Zero(t, *updated.Layers[2].HiddenBytes)
	}
	img, _ := c.nextHiddenBytesImage(time.Now())
	assert.Nil(t, img)
	assert.Equal(t, 4, scans)
	assert.Len(t, store.events, 12)
}

func TestHiddenBytesPreservesConcurrentMetadata(t *testing.T) {
	img := hiddenBytesTestImage("updated")
	var c *collector
	c, _ = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		updated := *img
		updated.RepoTags = []string{"new-tag"}
		updated.SBOM = &workloadmeta.CompressedSBOM{Status: workloadmeta.Success, GenerationMethod: "overlayfs"}
		publishHiddenBytesTestImage(c, &updated)
		return hiddenBytesTestResult(img), nil
	})
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	updated := c.latestImages[img.ID]
	assert.Equal(t, []string{"new-tag"}, updated.RepoTags)
	require.NotNil(t, updated.SBOM)
	assert.Equal(t, workloadmeta.Success, updated.SBOM.Status)
	assert.Nil(t, img.Layers[0].HiddenBytes, "previously published entity must remain immutable")
	assert.NotNil(t, updated.Layers[0].HiddenBytes)
}

func TestHiddenBytesRecoversAfterMetadataLoss(t *testing.T) {
	for _, afterCompletion := range []bool{false, true} {
		t.Run(fmt.Sprintf("after-completion=%t", afterCompletion), func(t *testing.T) {
			img := hiddenBytesTestImage("metadata-recovery")
			var c *collector
			scans := 0
			c, _ = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
				scans++
				if !afterCompletion && scans == 1 {
					incomplete := *img
					incomplete.Layers = nil
					publishHiddenBytesTestImage(c, &incomplete)
				}
				return hiddenBytesTestResult(img), nil
			})
			publishHiddenBytesTestImage(c, img)
			request, job := c.nextHiddenBytesImage(time.Now())
			c.collectHiddenBytes(t.Context(), request, job)
			if afterCompletion {
				incomplete := *img
				incomplete.Layers = nil
				publishHiddenBytesTestImage(c, &incomplete)
			}
			assert.False(t, c.hiddenBytes.jobs[img.ID].finished)
			publishHiddenBytesTestImage(c, img)
			request, job = c.nextHiddenBytesImage(time.Now().Add(time.Hour))
			require.NotNil(t, request)
			c.collectHiddenBytes(t.Context(), request, job)
			require.NotNil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
			assert.Equal(t, uint64(10), *c.latestImages[img.ID].Layers[0].HiddenBytes)
			if afterCompletion {
				assert.Equal(t, 1, scans, "recover completed results from cache")
			} else {
				assert.Equal(t, 2, scans)
			}
		})
	}
}

func TestHiddenBytesDeleteDuringScan(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(fmt.Sprintf("recreate=%t", recreate), func(t *testing.T) {
			img := hiddenBytesTestImage("deleted")
			var c *collector
			var store *hiddenBytesTestStore
			c, store = newHiddenBytesTestCollector(func(_ context.Context, _ *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
				c.handleImagesMut.Lock()
				delete(c.latestImages, img.ID)
				c.forgetHiddenBytesImageLocked(img.ID)
				c.handleImagesMut.Unlock()
				if recreate {
					publishHiddenBytesTestImage(c, img)
				}
				return hiddenBytesTestResult(img), nil
			})
			publishHiddenBytesTestImage(c, img)
			request, job := c.nextHiddenBytesImage(time.Now())
			c.collectHiddenBytes(t.Context(), request, job)
			if recreate {
				assert.Len(t, store.events, 2)
				assert.Nil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
				assert.NotSame(t, job, c.hiddenBytes.jobs[img.ID])
			} else {
				assert.Len(t, store.events, 1)
				assert.Empty(t, c.latestImages)
				assert.Empty(t, c.hiddenBytes.jobs)
			}
		})
	}
}

func TestHiddenBytesRetryBounds(t *testing.T) {
	c, store := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		return nil, errors.New("snapshots not available")
	})
	img := hiddenBytesTestImage("retry")
	publishHiddenBytesTestImage(c, img)
	for i := 0; i < hiddenBytesMaxAttempts; i++ {
		request, job := c.nextHiddenBytesImage(time.Now().Add(time.Hour))
		require.NotNil(t, request)
		c.collectHiddenBytes(t.Context(), request, job)
		immediate, _ := c.nextHiddenBytesImage(time.Now())
		assert.Nil(t, immediate, "retry must respect backoff")
	}
	request, _ := c.nextHiddenBytesImage(time.Now().Add(time.Hour))
	assert.Nil(t, request)
	assert.Len(t, store.events, 1, "failures must not publish zero-byte results")
	assert.Empty(t, c.hiddenBytes.cache.entries)
	// Self-publication/SBOM update does not reset exhausted retries.
	publishHiddenBytesTestImage(c, img)
	assert.Equal(t, hiddenBytesMaxAttempts, c.hiddenBytes.jobs[img.ID].attempts)
	c.handleImagesMut.Lock()
	c.retryHiddenBytesImageLocked(img.ID)
	c.handleImagesMut.Unlock()
	request, _ = c.nextHiddenBytesImage(time.Now())
	assert.Nil(t, request, "a runtime event retains the last backoff")
	request, _ = c.nextHiddenBytesImage(time.Now().Add(time.Hour))
	assert.NotNil(t, request)
}

func TestHiddenBytesCacheAvoidsScan(t *testing.T) {
	c, _ := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		t.Fatal("cached image should not be scanned")
		return nil, nil
	})
	img := hiddenBytesTestImage("cached")
	require.NoError(t, c.hiddenBytes.cache.put(img.ID, hiddenBytesTestResult(img)))
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	require.NotNil(t, c.latestImages[img.ID].Layers[0].HiddenBytes)
	assert.Equal(t, uint64(10), *c.latestImages[img.ID].Layers[0].HiddenBytes)
}

func TestHiddenBytesOrderedResults(t *testing.T) {
	img := hiddenBytesTestImage("repeated")
	img.Layers[2].DiffID = img.Layers[0].DiffID
	results := hiddenBytesTestResult(img)
	assert.True(t, hiddenResultsMatch(img, results))
	c, _ := newHiddenBytesTestCollector(func(context.Context, *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		return results, nil
	})
	publishHiddenBytesTestImage(c, img)
	request, job := c.nextHiddenBytesImage(time.Now())
	c.collectHiddenBytes(t.Context(), request, job)
	updated := c.latestImages[img.ID]
	assert.Equal(t, uint64(10), *updated.Layers[0].HiddenBytes)
	assert.Zero(t, *updated.Layers[2].HiddenBytes)
	assert.False(t, hiddenResultsMatch(img, results[:1]))
	img.Layers[0].DiffID = ""
	assert.False(t, hiddenResultsMatch(img, results))
}

func TestHiddenBytesShutdownCancelsScan(t *testing.T) {
	started := make(chan struct{})
	c, store := newHiddenBytesTestCollector(func(ctx context.Context, _ *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	publishHiddenBytesTestImage(c, hiddenBytesTestImage("shutdown"))
	c.startHiddenBytesCollection(t.Context())
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("scan did not start")
	}
	c.stopHiddenBytesCollection()
	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.events, 1)
	assert.Empty(t, c.hiddenBytes.cache.entries)
}

func TestHiddenBytesDisabledHasNoState(t *testing.T) {
	c := &collector{cfg: config.NewMockWithOverrides(t, map[string]interface{}{
		"container_image.enabled":              true,
		"container_image.hidden_bytes.enabled": false,
	}), store: &hiddenBytesTestStore{}}
	c.initHiddenBytesCollection()
	c.startHiddenBytesCollection(t.Context())
	defer c.stopHiddenBytesCollection()
	publishHiddenBytesTestImage(c, hiddenBytesTestImage("disabled"))
	assert.Nil(t, c.hiddenBytes)
	assert.Nil(t, c.latestImages)
}

func TestPreserveHiddenBytesRequiresMatchingOrderedLayers(t *testing.T) {
	src := hiddenBytesTestImage("image")
	value := uint64(42)
	src.Layers[0].HiddenBytes = &value
	dst := hiddenBytesTestImage("image")
	preserveHiddenBytes(dst, src)
	require.NotNil(t, dst.Layers[0].HiddenBytes)
	assert.Equal(t, value, *dst.Layers[0].HiddenBytes)
	dst = hiddenBytesTestImage("image")
	dst.Layers[2].DiffID = digest.FromString("different").String()
	preserveHiddenBytes(dst, src)
	assert.Nil(t, dst.Layers[0].HiddenBytes, "must not attach partial results to a mismatched stack")
}

type hiddenBytesConfigImage struct {
	containerd.Image
	config func(context.Context) (ocispec.Descriptor, error)
}

func (i hiddenBytesConfigImage) Config(ctx context.Context) (ocispec.Descriptor, error) {
	return i.config(ctx)
}

func TestHiddenBytesRejectsMovedTag(t *testing.T) {
	meta := hiddenBytesTestImage("image:latest")
	c := &collector{containerdClient: &fake.MockedContainerdClient{
		MockImage: func(namespace, name string) (containerd.Image, error) {
			assert.Equal(t, meta.Namespace, namespace)
			assert.Equal(t, meta.Name, name)
			return hiddenBytesConfigImage{config: func(ctx context.Context) (ocispec.Descriptor, error) {
				namespace, ok := namespaces.Namespace(ctx)
				assert.True(t, ok)
				assert.Equal(t, meta.Namespace, namespace)
				return ocispec.Descriptor{Digest: digest.FromString("new-image")}, nil
			}}, nil
		},
	}}
	result, err := c.scanImageHiddenBytes(t.Context(), meta)
	require.ErrorContains(t, err, "no longer points to")
	assert.Nil(t, result)
}
