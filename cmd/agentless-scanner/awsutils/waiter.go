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

// WaitResult is the result of a snapshot creation wait.
type WaitResult struct {
	Err      error
	Snapshot *ec2types.Snapshot
}

type waitSub chan WaitResult

// SnapshotWaiter allows to wait for multiple snapshot creation at once.
type SnapshotWaiter struct {
	sync.Mutex
	subs map[string]map[string][]waitSub
}

// Wait waits for the given snapshot to be created and returns a channel that
// will send an error or nil.
func (w *SnapshotWaiter) Wait(ctx context.Context, snapshotID types.CloudID, ec2client *ec2.Client) <-chan WaitResult {
	w.Lock()
	defer w.Unlock()
	region := snapshotID.Region()
	if w.subs == nil {
		w.subs = make(map[string]map[string][]waitSub)
	}
	if w.subs[region] == nil {
		w.subs[region] = make(map[string][]waitSub)
	}
	ch := make(chan WaitResult, 1)
	subs := w.subs[region]
	subs[snapshotID.ResourceName()] = append(subs[snapshotID.ResourceName()], ch)
	if len(subs) == 1 {
		go w.loop(ctx, region, ec2client)
	}
	return ch
}

func (w *SnapshotWaiter) abort(region string, err error) {
	w.Lock()
	defer w.Unlock()
	for _, subs := range w.subs[region] {
		for _, waitSub := range subs {
			waitSub <- WaitResult{Err: err}
		}
	}
	w.subs[region] = nil
}

func (w *SnapshotWaiter) loop(ctx context.Context, region string, ec2client *ec2.Client) {
	const (
		tickerInterval  = 5 * time.Second
		snapshotTimeout = 15 * time.Minute
	)

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
		case <-ctx.Done():
			w.abort(region, ctx.Err())
			return
		}

		w.Lock()
		snapshotIDs := make([]string, 0, len(w.subs[region]))
		for snapshotID := range w.subs[region] {
			snapshotIDs = append(snapshotIDs, snapshotID)
		}
		w.Unlock()

		if len(snapshotIDs) == 0 {
			return
		}

		// TODO: could we rely on ListSnapshotBlocks instead of
		// DescribeSnapshots as a "fast path" to not consume precious quotas ?
		output, err := ec2client.DescribeSnapshots(context.TODO(), &ec2.DescribeSnapshotsInput{
			SnapshotIds: snapshotIDs,
		})
		if err != nil {
			w.abort(region, err)
			return
		}

		snapshots := make(map[string]ec2types.Snapshot, len(output.Snapshots))
		for _, snap := range output.Snapshots {
			snapshots[*snap.SnapshotId] = snap
		}

		w.Lock()
		subs := w.subs[region]
		noError := errors.New("")
		for _, snapshotID := range snapshotIDs {
			var errp error
			snap, ok := snapshots[snapshotID]
			if !ok {
				errp = fmt.Errorf("snapshot %q does not exist", *snap.SnapshotId)
			} else {
				switch snap.State {
				case ec2types.SnapshotStatePending:
					if elapsed := time.Since(*snap.StartTime); elapsed > snapshotTimeout {
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
					errp = noError
				}
			}
			if errp != nil {
				for _, ch := range subs[*snap.SnapshotId] {
					if errp == noError {
						ch <- WaitResult{Snapshot: &snap}
					} else {
						ch <- WaitResult{Err: errp}
					}
				}
				delete(subs, *snap.SnapshotId)
			}
		}
		w.Unlock()
	}
}
