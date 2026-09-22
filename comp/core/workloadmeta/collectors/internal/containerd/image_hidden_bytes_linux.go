// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	"github.com/containerd/containerd/v2/pkg/namespaces"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	hiddenBytesScanTimeout   = time.Minute
	hiddenBytesSweepInterval = time.Second
	hiddenBytesMaxAttempts   = 3
	hiddenBytesRetryDelay    = 30 * time.Second
)

type hiddenLayerResult struct {
	DiffID string `json:"diff_id"`
	Bytes  uint64 `json:"bytes"`
}

type hiddenBytesScan func(context.Context, *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error)

type hiddenBytesJob struct {
	sequence    uint64
	attempts    int
	nextAttempt time.Time
	finished    bool
}

// Jobs only hold scheduling metadata for currently known images. A single
// worker discovers jobs from this inventory, so a full wake channel cannot drop
// an image and there is no unbounded work queue or per-image goroutine.
type hiddenBytesCollector struct {
	wake     chan struct{}
	done     chan struct{}
	cancel   context.CancelFunc
	scan     hiddenBytesScan
	jobs     map[string]*hiddenBytesJob // guarded by handleImagesMut
	sequence uint64
	cache    *hiddenBytesCache // owned by the worker
}

func (c *collector) initHiddenBytesCollection() {
	if !c.imageMetadataCollectionIsEnabled() || !c.cfg.GetBool("container_image.hidden_bytes.enabled") {
		return
	}
	cache := newHiddenBytesCache(filepath.Join(c.cfg.GetString("run_path"), "container-image-hidden-bytes-v1.json"))
	if err := cache.load(); err != nil {
		log.Debugf("Ignoring unavailable container image hidden-bytes cache: %v", err)
	}
	c.hiddenBytes = &hiddenBytesCollector{
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
		scan:  c.scanImageHiddenBytes,
		jobs:  make(map[string]*hiddenBytesJob),
		cache: cache,
	}
}

func (c *collector) startHiddenBytesCollection(ctx context.Context) {
	if c.hiddenBytes == nil {
		return
	}
	ctx, c.hiddenBytes.cancel = context.WithCancel(ctx)
	go c.runHiddenBytesCollection(ctx)
}

func (c *collector) stopHiddenBytesCollection() {
	if c.hiddenBytes == nil || c.hiddenBytes.cancel == nil {
		return
	}
	c.hiddenBytes.cancel()
	<-c.hiddenBytes.done
}

func (c *collector) enqueueHiddenBytesImageLocked(img *workloadmeta.ContainerImageMetadata) {
	h := c.hiddenBytes
	if h == nil {
		return
	}
	if job, exists := h.jobs[img.ID]; !exists {
		h.sequence++
		h.jobs[img.ID] = &hiddenBytesJob{sequence: h.sequence}
	} else if job.finished && !hasCompleteHiddenBytes(img) {
		// A transient config-read failure can remove layer metadata from a
		// runtime update. Recover the completed result from cache once the
		// metadata is available again instead of permanently losing it.
		job.finished = false
	}
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

func hasCompleteHiddenBytes(img *workloadmeta.ContainerImageMetadata) bool {
	found := false
	for _, layer := range img.Layers {
		if layer.History != nil && layer.History.EmptyLayer {
			continue
		}
		if layer.DiffID == "" || layer.HiddenBytes == nil {
			return false
		}
		found = true
	}
	return found
}

func (c *collector) forgetHiddenBytesImageLocked(id string) {
	if c.hiddenBytes != nil {
		delete(c.hiddenBytes.jobs, id)
	}
}

// A genuine runtime event can signal that a previously unavailable snapshot
// was unpacked. SBOM and hidden-byte result publications never reset retries.
func (c *collector) retryHiddenBytesImageLocked(id string) {
	if c.hiddenBytes == nil {
		return
	}
	job := c.hiddenBytes.jobs[id]
	if job != nil && job.attempts >= hiddenBytesMaxAttempts {
		job.attempts = 0
		// Preserve the last backoff even when runtime events arrive in a burst.
		c.hiddenBytes.sequence++
		job.sequence = c.hiddenBytes.sequence
	}
}

func (c *collector) runHiddenBytesCollection(ctx context.Context) {
	h := c.hiddenBytes
	defer close(h.done)
	ticker := time.NewTicker(hiddenBytesSweepInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		if img, job := c.nextHiddenBytesImage(time.Now()); img != nil {
			c.collectHiddenBytes(ctx, img, job)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-h.wake:
		case <-ticker.C:
		}
	}
}

func (c *collector) nextHiddenBytesImage(now time.Time) (*workloadmeta.ContainerImageMetadata, *hiddenBytesJob) {
	c.handleImagesMut.Lock()
	defer c.handleImagesMut.Unlock()
	var selected *hiddenBytesJob
	var img *workloadmeta.ContainerImageMetadata
	for id, job := range c.hiddenBytes.jobs {
		if job.finished || job.attempts >= hiddenBytesMaxAttempts || now.Before(job.nextAttempt) {
			continue
		}
		current := c.latestImages[id]
		if current == nil {
			continue
		}
		if selected == nil || job.sequence < selected.sequence {
			selected = job
			img = current
		}
	}
	if img == nil {
		return nil, nil
	}
	copy := *img
	copy.Layers = slices.Clone(img.Layers)
	return &copy, selected
}

func (c *collector) collectHiddenBytes(ctx context.Context, img *workloadmeta.ContainerImageMetadata, job *hiddenBytesJob) {
	h := c.hiddenBytes
	results, cached := h.cache.get(img.ID)
	if cached && !hiddenResultsMatch(img, results) {
		cached = false
	}
	if cached {
		log.Debugf("Container image hidden-byte cache hit for %s", img.ID)
	}
	var err error
	if !cached {
		scanCtx, cancel := context.WithTimeout(ctx, hiddenBytesScanTimeout)
		results, err = h.scan(scanCtx, img)
		cancel()
		if err == nil && !hiddenResultsMatch(img, results) {
			err = fmt.Errorf("hidden-byte results do not match ordered image layers")
		}
	}
	if ctx.Err() != nil {
		return
	}
	c.handleImagesMut.Lock()
	current := c.latestImages[img.ID]
	// A deletion followed by re-creation has a different job, even if its image
	// ID is unchanged. Never publish a result belonging to the old lifetime.
	if ctx.Err() != nil || current == nil || h.jobs[img.ID] != job {
		c.handleImagesMut.Unlock()
		return
	}
	if err == nil && !hiddenResultsMatch(current, results) {
		err = fmt.Errorf("current image metadata no longer matches hidden-byte results")
	}
	if err != nil {
		job.attempts++
		job.nextAttempt = time.Now().Add(hiddenBytesRetryDelay * time.Duration(1<<(job.attempts-1)))
		attempts := job.attempts
		c.handleImagesMut.Unlock()
		log.Debugf("Container image hidden-byte scan unavailable for %s (attempt %d): %v", img.ID, attempts, err)
		return
	}
	updated := *current
	updated.Layers = slices.Clone(current.Layers)
	index := 0
	for i := range updated.Layers {
		if updated.Layers[i].History != nil && updated.Layers[i].History.EmptyLayer {
			continue
		}
		value := results[index].Bytes
		updated.Layers[i].HiddenBytes = &value
		index++
	}
	job.finished = true
	c.publishImageLocked(&updated)
	c.handleImagesMut.Unlock()
	if !cached {
		log.Debugf("Container image hidden-byte scan completed for %s (%d layers)", img.ID, len(results))
		if err := h.cache.put(img.ID, results); err != nil {
			log.Debugf("Unable to persist container image hidden-byte result: %v", err)
		}
	}
}

func hiddenResultsMatch(img *workloadmeta.ContainerImageMetadata, results []hiddenLayerResult) bool {
	index := 0
	for _, layer := range img.Layers {
		if layer.History != nil && layer.History.EmptyLayer {
			continue
		}
		if layer.DiffID == "" || index >= len(results) || layer.DiffID != results[index].DiffID {
			return false
		}
		index++
	}
	return index > 0 && index == len(results)
}

func (c *collector) scanImageHiddenBytes(ctx context.Context, meta *workloadmeta.ContainerImageMetadata) ([]hiddenLayerResult, error) {
	ctx = namespaces.WithNamespace(ctx, meta.Namespace)
	img, err := c.containerdClient.Image(meta.Namespace, meta.Name)
	if err != nil {
		return nil, err
	}
	config, err := img.Config(ctx)
	if err != nil {
		return nil, err
	}
	if config.Digest.String() != meta.ID {
		return nil, fmt.Errorf("image reference no longer points to %s", meta.ID)
	}
	layers, cleanup, err := cutil.AcquireImageLayers(ctx, c.containerdClient, meta.Namespace, img, hiddenBytesScanTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := cleanup(ctx); err != nil {
			log.Debugf("Unable to clean up hidden-byte image snapshot: %v", err)
		}
	}()
	bytes, err := cutil.CalculateHiddenBytes(ctx, layers, cutil.DefaultHiddenBytesLimits())
	if err != nil {
		return nil, err
	}
	if len(bytes) != len(layers) {
		return nil, fmt.Errorf("hidden-byte result count does not match image layers")
	}
	results := make([]hiddenLayerResult, len(layers))
	for i, layer := range layers {
		results[i] = hiddenLayerResult{DiffID: layer.DiffID, Bytes: bytes[i]}
	}
	return results, nil
}
