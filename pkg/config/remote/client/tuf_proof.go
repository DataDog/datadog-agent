// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"slices"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// GetConfigTUFProof returns a defensive copy of the current Director proof for
// targetPath. It returns false until the target and signed Targets metadata have
// both been received from the Core Agent.
func (c *Client) GetConfigTUFProof(targetPath string) (state.ConfigTUFProof, bool) {
	c.proofMu.RLock()
	defer c.proofMu.RUnlock()

	targetFile, ok := c.proofTargetFiles[targetPath]
	if !ok || len(c.proofTargets) == 0 {
		return state.ConfigTUFProof{}, false
	}
	return state.ConfigTUFProof{
		Roots:      cloneByteSlices(c.proofRoots),
		Targets:    slices.Clone(c.proofTargets),
		TargetPath: targetPath,
		TargetFile: slices.Clone(targetFile),
	}, true
}

// storeConfigTUFProof retains the complete proof chain from the bootstrap root
// reported by this downstream client. It is called only after the corresponding
// repository update has been accepted.
func (c *Client) storeConfigTUFProof(update *pbgo.ClientGetConfigsResponse) {
	if len(update.Targets) == 0 {
		return
	}

	c.proofMu.Lock()
	defer c.proofMu.Unlock()

	for _, root := range update.Roots {
		c.proofRoots = append(c.proofRoots, slices.Clone(root))
	}
	c.proofTargets = slices.Clone(update.Targets)

	currentTargets := make(map[string]struct{}, len(update.ClientConfigs))
	for _, targetPath := range update.ClientConfigs {
		currentTargets[targetPath] = struct{}{}
	}
	for targetPath := range c.proofTargetFiles {
		if _, ok := currentTargets[targetPath]; !ok {
			delete(c.proofTargetFiles, targetPath)
		}
	}
	for _, targetFile := range update.TargetFiles {
		c.proofTargetFiles[targetFile.Path] = slices.Clone(targetFile.Raw)
	}
}

func cloneByteSlices(values [][]byte) [][]byte {
	cloned := make([][]byte, len(values))
	for i, value := range values {
		cloned[i] = slices.Clone(value)
	}
	return cloned
}
