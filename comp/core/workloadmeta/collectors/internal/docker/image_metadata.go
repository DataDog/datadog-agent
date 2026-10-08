// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"sync"

	"github.com/moby/moby/api/types/image"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Bound pressure on the Docker daemon while avoiding a serial inspect/history
// round trip for every local image during workloadmeta startup.
const initialImageMetadataWorkers = 4

// collectInitialImageEvents waits for the complete image snapshot before returning.
// Container discovery depends on these images for repository digest resolution.
// Workers only fetch metadata; publishing remains serial and in Docker list order.
func collectInitialImageEvents(
	ctx context.Context,
	images []image.Summary,
	getMetadata func(context.Context, string) (*workloadmeta.ContainerImageMetadata, error),
) ([]workloadmeta.CollectorEvent, error) {
	type result struct {
		metadata *workloadmeta.ContainerImageMetadata
		err      error
	}
	results := make([]result, len(images))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(initialImageMetadataWorkers, len(images)) {
		workers.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				results[i].metadata, results[i].err = getMetadata(ctx, images[i].ID)
			}
		})
	}

dispatch:
	for i := range images {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	events := make([]workloadmeta.CollectorEvent, 0, len(images))
	for _, result := range results {
		if result.err != nil {
			// An image can disappear between listing and inspection. Preserve the
			// existing best-effort behavior for individual lookup failures.
			log.Warnf("%s", result.err.Error())
			continue
		}
		events = append(events, workloadmeta.CollectorEvent{
			Source: workloadmeta.SourceRuntime,
			Type:   workloadmeta.EventTypeSet,
			Entity: result.metadata,
		})
	}
	return events, nil
}
