// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package awsutils provides some utility functions and types for operating
// with AWS services.
package awsutils

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/agentless-scanner/types"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	waitTickerInterval  = 5 * time.Second
	waitSnapshotTimeout = 15 * time.Minute
	waitImageTimeout    = 30 * time.Second
)

var errWaitDone = errors.New("")

// WaitResult is the result of a snapshot creation wait.
type WaitResult struct {
	Err      error
	Snapshot *ec2types.Snapshot
	Image    *ec2types.Image
}

type waitSub chan WaitResult

type waitGroup struct {
	region    string
	accountID string
}

// ResourceWaiter allows to wait for multiple snapshot creation at once.
type ResourceWaiter struct {
	sync.Mutex
	subs map[waitGroup]map[types.CloudID][]waitSub
}

// Wait waits for the given snapshot or image to be created and returns a
// channel that will send an error or nil.
func (w *ResourceWaiter) Wait(ctx context.Context, resourceID types.CloudID, ec2client *ec2.Client) <-chan WaitResult {
	w.Lock()
	defer w.Unlock()

	ch := make(chan WaitResult, 1)
	if resourceID.ResourceType() != types.ResourceTypeSnapshot &&
		resourceID.ResourceType() != types.ResourceTypeHostImage {
		ch <- WaitResult{Err: fmt.Errorf("unsupported resource type %q", resourceID.ResourceType())}
		return ch
	}
	group := waitGroup{
		region:    resourceID.Region(),
		accountID: resourceID.AccountID(),
	}
	if w.subs == nil {
		w.subs = make(map[waitGroup]map[types.CloudID][]waitSub)
	}
	if w.subs[group] == nil {
		w.subs[group] = make(map[types.CloudID][]waitSub)
	}
	subs := w.subs[group]
	subs[resourceID] = append(subs[resourceID], ch)
	if len(subs) == 1 {
		go w.loop(ctx, group, ec2client)
	}
	return ch
}

func (w *ResourceWaiter) abort(group waitGroup, err error, resourceType ...types.ResourceType) bool {
	w.Lock()
	defer w.Unlock()
	for resourceID, subs := range w.subs[group] {
		if len(resourceType) == 0 || resourceID.ResourceType() == resourceType[0] {
			for _, sub := range subs {
				sub <- WaitResult{Err: err}
			}
			delete(w.subs[group], resourceID)
		}
	}
	if len(w.subs[group]) == 0 {
		delete(w.subs, group)
		return true
	}
	return false
}

func (w *ResourceWaiter) loop(ctx context.Context, group waitGroup, ec2client *ec2.Client) {
	ticker := time.NewTicker(waitTickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			w.abort(group, ctx.Err())
			return
		}

		w.Lock()
		var snapshotIDs []string
		var imageIDs []string
		for resourceID := range w.subs[group] {
			switch resourceID.ResourceType() {
			case types.ResourceTypeSnapshot:
				snapshotIDs = append(snapshotIDs, resourceID.ResourceName())
			case types.ResourceTypeHostImage:
				imageIDs = append(imageIDs, resourceID.ResourceName())
			}
		}
		w.Unlock()

		done := len(snapshotIDs) == 0 && len(imageIDs) == 0
		if !done {
			if len(snapshotIDs) > 0 {
				done = w.pollSnapshots(ctx, group, ec2client, snapshotIDs)
			}
			if len(imageIDs) > 0 {
				done = w.pollImages(ctx, group, ec2client, imageIDs)
			}
		}
		if done {
			return
		}
	}
}

func (w *ResourceWaiter) pollSnapshots(ctx context.Context, group waitGroup, ec2client *ec2.Client, snapshotIDs []string) bool {
	// TODO: could we rely on ListSnapshotBlocks instead of
	// DescribeSnapshots as a "fast path" to not consume precious quotas ?
	output, err := ec2client.DescribeSnapshots(ctx, &ec2.DescribeSnapshotsInput{
		SnapshotIds: snapshotIDs,
	})
	if err != nil {
		return w.abort(group, err, types.ResourceTypeSnapshot)
	}

	snapshots := make(map[string]ec2types.Snapshot, len(output.Snapshots))
	for _, snap := range output.Snapshots {
		snapshots[*snap.SnapshotId] = snap
	}

	w.Lock()
	defer w.Unlock()
	subs := w.subs[group]
	for _, snapshotID := range snapshotIDs {
		var errp error
		snap, ok := snapshots[snapshotID]
		if !ok {
			errp = fmt.Errorf("snapshot %q does not exist", *snap.SnapshotId)
		} else {
			switch snap.State {
			case ec2types.SnapshotStatePending:
				if elapsed := time.Since(*snap.StartTime); elapsed > waitSnapshotTimeout {
					errp = fmt.Errorf("snapshot %q creation timed out (started at %s)", *snap.SnapshotId, *snap.StartTime)
				}
			case ec2types.SnapshotStateRecoverable:
				errp = fmt.Errorf("snapshot %q in recoverable state", *snap.SnapshotId)
			case ec2types.SnapshotStateRecovering:
				errp = fmt.Errorf("snapshot %q in recovering state", *snap.SnapshotId)
			case ec2types.SnapshotStateError:
				msg := fmt.Sprintf("snapshot %q failed", *snap.SnapshotId)
				if snap.StateMessage != nil {
					msg += ": " + *snap.StateMessage
				}
				errp = fmt.Errorf(msg)
			case ec2types.SnapshotStateCompleted:
				errp = errWaitDone
			}
		}
		if errp != nil {
			snapshotID, _ := types.AWSCloudID(group.region, group.accountID, types.ResourceTypeSnapshot, *snap.SnapshotId)
			for _, sub := range subs[snapshotID] {
				if errp == errWaitDone {
					sub <- WaitResult{Snapshot: &snap}
				} else {
					sub <- WaitResult{Err: errp}
				}
			}
			delete(subs, snapshotID)
		}
	}
	if len(subs) == 0 {
		delete(w.subs, group)
		return true
	}
	return false
}

func (w *ResourceWaiter) pollImages(ctx context.Context, group waitGroup, ec2client *ec2.Client, imageIDs []string) bool {
	output, err := ec2client.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds: imageIDs,
	})
	if err != nil {
		return w.abort(group, err, types.ResourceTypeHostImage)
	}
	images := make(map[string]ec2types.Image, len(output.Images))
	for _, image := range output.Images {
		images[*image.ImageId] = image
	}

	w.Lock()
	defer w.Unlock()
	subs := w.subs[group]
	for _, imageID := range imageIDs {
		image, ok := images[imageID]
		var errp error
		if !ok {
			errp = fmt.Errorf("image %q does not exist", imageID)
		} else {
			switch image.State {
			case ec2types.ImageStatePending:
				creationDate, err := time.Parse(time.RFC3339, *image.CreationDate)
				if err != nil {
					errp = fmt.Errorf("invalid creation date %q for image %q: %w", *image.CreationDate, imageID, err)
				} else if elapsed := time.Since(creationDate); elapsed > waitImageTimeout {
					errp = fmt.Errorf("image %q creation timed out (started at %s)", imageID, *image.CreationDate)
				}
			case ec2types.ImageStateAvailable:
				errp = errWaitDone
			case ec2types.ImageStateInvalid,
				ec2types.ImageStateDeregistered,
				ec2types.ImageStateTransient,
				ec2types.ImageStateFailed,
				ec2types.ImageStateError,
				ec2types.ImageStateDisabled:
				errp = fmt.Errorf("image %q in invalid state: %s", imageID, image.State)
			}
		}
		if errp != nil {
			imageID, _ := types.AWSCloudID(group.region, group.accountID, types.ResourceTypeHostImage, *image.ImageId)
			for _, sub := range subs[imageID] {
				if errp == errWaitDone {
					sub <- WaitResult{Image: &image}
				} else {
					sub <- WaitResult{Err: errp}
				}
			}
			delete(subs, imageID)
		}
	}
	if len(subs) == 0 {
		delete(w.subs, group)
		return true
	}
	return false
}
