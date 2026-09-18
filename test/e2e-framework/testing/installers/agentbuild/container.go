// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentbuild

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"
)

// RunContainer owns cleanup even if cancellation kills the Docker CLI before
// the daemon removes its container. No unbounded nested build is left running.
func (a Adapter) runContainer(ctx context.Context, dir string, args []string) ([]byte, error) {
	return a.runContainerOutput(ctx, dir, args, false)
}
func (a Adapter) runBuildContainer(ctx context.Context, dir string, args []string) ([]byte, error) {
	return a.runContainerOutput(ctx, dir, args, true)
}
func (a Adapter) runContainerOutput(ctx context.Context, dir string, args []string, stream bool) ([]byte, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	name := "e2ectl-prepare-" + hex.EncodeToString(nonce[:])
	args = append([]string{"run", "--name", name}, args...)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, _ = a.run(cleanup, "", "docker", "rm", "-f", name)
	}()
	return a.execute(ctx, Invocation{Program: "docker", Args: args, Dir: dir, StreamOutput: stream})
}
