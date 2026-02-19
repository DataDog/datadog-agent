// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

import "encoding/json"

// PatchIntent describes a set of mutations to apply to a single Kubernetes
// resource. It combines a Target with one or more PatchOperations.
//
// Use NewPatchIntent to create one, chain With() calls to add operations,
// then pass it to Patcher.Apply().
type PatchIntent struct {
	target     Target
	operations []PatchOperation
}

// NewPatchIntent creates a PatchIntent for the given target.
func NewPatchIntent(target Target) *PatchIntent {
	return &PatchIntent{target: target}
}

// With appends an operation to the intent and returns the intent for chaining.
func (p *PatchIntent) With(op PatchOperation) *PatchIntent {
	p.operations = append(p.operations, op)
	return p
}

// Target returns the target resource for this intent.
func (p *PatchIntent) Target() Target {
	return p.target
}

// Build constructs the final patch body by deep-merging all operations.
// Returns the JSON-encoded patch bytes suitable for a Kubernetes Patch() call.
// Returns nil if there are no operations.
func (p *PatchIntent) Build() ([]byte, error) {
	if len(p.operations) == 0 {
		return nil, nil
	}

	merged := make(map[string]interface{})
	for _, op := range p.operations {
		fragment := op.build()
		mergeMaps(merged, fragment)
	}

	return json.Marshal(merged)
}

// mergeMaps recursively merges src into dst. When both src and dst have a
// map[string]interface{} at the same key, the maps are merged recursively.
// Otherwise the src value overwrites the dst value.
func mergeMaps(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		dstVal, exists := dst[key]
		if !exists {
			dst[key] = srcVal
			continue
		}

		// If both values are maps, merge recursively
		dstMap, dstOk := dstVal.(map[string]interface{})
		srcMap, srcOk := srcVal.(map[string]interface{})
		if dstOk && srcOk {
			mergeMaps(dstMap, srcMap)
			continue
		}

		// Otherwise overwrite
		dst[key] = srcVal
	}
}
