// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/moby/moby/api/types/image"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func startupImages(count int) []image.Summary {
	images := make([]image.Summary, count)
	for i := range images {
		images[i].ID = fmt.Sprintf("image-%d", i)
	}
	return images
}

func startupImageMetadata(id string) *workloadmeta.ContainerImageMetadata {
	return &workloadmeta.ContainerImageMetadata{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: id},
	}
}

func TestCollectInitialImageEventsBoundedAndOrdered(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	images := startupImages(initialImageMetadataWorkers * 2)
	started := make(chan string, len(images))
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	var active, peak atomic.Int32
	type outcome struct {
		events []workloadmeta.CollectorEvent
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		events, err := collectInitialImageEvents(ctx, images, func(ctx context.Context, id string) (*workloadmeta.ContainerImageMetadata, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			started <- id
			select {
			case <-release:
				return startupImageMetadata(id), nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		})
		finished <- outcome{events, err}
	}()

	// All workers must enter concurrently; none can take a second job until released.
	for range initialImageMetadataWorkers {
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("metadata lookups did not run concurrently")
		}
	}
	require.EqualValues(t, initialImageMetadataWorkers, active.Load())
	select {
	case <-started:
		t.Fatal("worker limit exceeded")
	case <-finished:
		t.Fatal("returned before all metadata lookups completed")
	default:
	}
	unblock()
	select {
	case result := <-finished:
		require.NoError(t, result.err)
		require.Len(t, result.events, len(images))
		for i, event := range result.events {
			require.Equal(t, images[i].ID, event.Entity.GetID().ID)
			require.Equal(t, workloadmeta.SourceRuntime, event.Source)
			require.Equal(t, workloadmeta.EventTypeSet, event.Type)
		}
	case <-ctx.Done():
		t.Fatal("metadata lookups did not finish")
	}
	require.Zero(t, active.Load())
	require.EqualValues(t, initialImageMetadataWorkers, peak.Load())
}

func TestCollectInitialImageEventsSkipsFailedImages(t *testing.T) {
	events, err := collectInitialImageEvents(t.Context(), startupImages(4), func(_ context.Context, id string) (*workloadmeta.ContainerImageMetadata, error) {
		if id == "image-1" {
			return nil, errors.New("image was removed")
		}
		return startupImageMetadata(id), nil
	})
	require.NoError(t, err)
	require.Len(t, events, 3)
	for i, id := range []string{"image-0", "image-2", "image-3"} {
		require.Equal(t, id, events[i].Entity.GetID().ID)
	}
}

func TestCollectInitialImageEventsCancellationJoinsWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	started := make(chan struct{}, initialImageMetadataWorkers)
	var active atomic.Int32
	finished := make(chan error, 1)
	go func() {
		events, err := collectInitialImageEvents(ctx, startupImages(10), func(ctx context.Context, _ string) (*workloadmeta.ContainerImageMetadata, error) {
			active.Add(1)
			defer active.Add(-1)
			started <- struct{}{}
			<-ctx.Done()
			return nil, ctx.Err()
		})
		if len(events) != 0 {
			finished <- errors.New("published a partial snapshot on cancellation")
			return
		}
		finished <- err
	}()
	deadline, stop := context.WithTimeout(t.Context(), 5*time.Second)
	defer stop()
	for range initialImageMetadataWorkers {
		select {
		case <-started:
		case <-deadline.Done():
			t.Fatal("metadata lookups did not start")
		}
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-deadline.Done():
		t.Fatal("canceled lookups did not finish")
	}
	require.Zero(t, active.Load(), "workers must exit before startup returns")
}

func TestCollectInitialImageEventsEmptyAndCanceled(t *testing.T) {
	lookup := func(context.Context, string) (*workloadmeta.ContainerImageMetadata, error) {
		t.Error("unexpected metadata lookup")
		return nil, errors.New("unexpected lookup")
	}
	events, err := collectInitialImageEvents(t.Context(), nil, lookup)
	require.NoError(t, err)
	require.Empty(t, events)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	events, err = collectInitialImageEvents(ctx, startupImages(10), lookup)
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, events)
}
