// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package azurebackend

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
)

// WaitResult is the result of a snapshot creation wait.
type WaitResult struct {
	Err      error
	Snapshot *armcompute.Snapshot
}

type waitSub chan WaitResult

// ResourceWaiter allows to wait for multiple snapshot creation at once.
type ResourceWaiter struct {
	sync.Mutex
	subs map[types.CloudID][]waitSub
}

// Wait waits for the given snapshot to be created and returns a
// channel that will send an error or nil.
func (w *ResourceWaiter) Wait(ctx context.Context, resourceID types.CloudID, poller *runtime.Poller[armcompute.SnapshotsClientCreateOrUpdateResponse]) <-chan WaitResult {
	w.Lock()
	defer w.Unlock()

	ch := make(chan WaitResult, 1)
	if resourceID.ResourceType() != types.ResourceTypeSnapshot {
		ch <- WaitResult{Err: fmt.Errorf("unsupported resource type %q", resourceID.ResourceType())}
		return ch
	}
	if w.subs == nil {
		w.subs = make(map[types.CloudID][]waitSub)
	}
	w.subs[resourceID] = append(w.subs[resourceID], ch)
	if len(w.subs[resourceID]) == 1 {
		go func() {
			resp, err := poller.PollUntilDone(ctx, nil)
			if err != nil {
				ch <- WaitResult{Err: err}
			} else {
				ch <- WaitResult{Snapshot: &resp.Snapshot}
			}
		}()
	}
	return ch
}
