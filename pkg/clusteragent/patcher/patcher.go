// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PatchOptions controls how a patch is applied.
type PatchOptions struct {
	// PatchType is the Kubernetes patch type to use.
	// Defaults to types.MergePatchType if zero-valued.
	PatchType types.PatchType

	// RetryOnConflict enables automatic retry with exponential backoff when
	// the API server returns a 409 Conflict (optimistic concurrency failure).
	RetryOnConflict bool

	// DryRun validates the patch without persisting it.
	DryRun bool

	// Caller identifies the subsystem making the patch, used for logging
	// (e.g. "language_detection", "rc_patcher", "vpa").
	Caller string

	// Subresource, if non-empty, routes the patch to the named subresource
	// (e.g. "resize" for in-place pod resource updates).
	Subresource string
}

// Patcher applies PatchIntents to Kubernetes resources via the dynamic client.
// It optionally enforces leader election so patches are only applied by the
// cluster agent leader.
type Patcher struct {
	client   dynamic.Interface
	isLeader func() bool
}

// NewPatcher creates a Patcher with the given dynamic client and a
// leader check function. If isLeader is non-nil, Apply will short-circuit
// with (false, nil) when the current instance is not the leader.
func NewPatcher(client dynamic.Interface, isLeader func() bool) *Patcher {
	return &Patcher{client: client, isLeader: isLeader}
}

// Apply executes a PatchIntent against the Kubernetes API server.
// It builds a patch from the intent's operations and applies it using the
// patch type specified in opts.PatchType (defaults to MergePatchType).
//
// Returns:
//   - true if the patch was applied (API call succeeded)
//   - false if the intent had no operations, or if not the leader (no-op)
//   - error if the patch could not be built or applied
func (p *Patcher) Apply(ctx context.Context, intent *PatchIntent, opts PatchOptions) (bool, error) {
	if p.isLeader != nil && !p.isLeader() {
		log.Debugf("[patcher/%s] not leader, skipping patch for %s", opts.Caller, intent.Target())
		return false, nil
	}

	patchData, err := intent.Build()
	if err != nil {
		return false, fmt.Errorf("failed to build patch: %w", err)
	}
	if patchData == nil {
		log.Debugf("[patcher/%s] no operations for %s, skipping", opts.Caller, intent.Target())
		return false, nil
	}

	patchType := opts.PatchType
	if patchType == "" {
		patchType = types.MergePatchType
	}

	target := intent.Target()
	log.Debugf("[patcher/%s] applying patch to %v", opts.Caller, target)

	applyFn := func() error {
		patchOpts := metav1.PatchOptions{}
		if opts.DryRun {
			patchOpts.DryRun = []string{metav1.DryRunAll}
		}

		var subresources []string
		if opts.Subresource != "" {
			subresources = append(subresources, opts.Subresource)
		}

		_, err := p.client.Resource(target.GVR).
			Namespace(target.Namespace).
			Patch(ctx, target.Name, patchType, patchData, patchOpts, subresources...)
		return err
	}

	if opts.RetryOnConflict {
		err = retry.RetryOnConflict(retry.DefaultRetry, applyFn)
	} else {
		err = applyFn()
	}

	if err != nil {
		return false, fmt.Errorf("failed to patch %s: %w", target, err)
	}

	log.Debugf("[patcher/%s] successfully patched %s", opts.Caller, target)
	return true, nil
}
