// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package executor

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
)

// testPipeCounter uniquifies named-pipe addresses across tests in the same process.
var testPipeCounter atomic.Uint64

// testListenAddr returns a unique named-pipe address; Windows has no Unix-socket equivalent,
// so the executor's transport listens on a named pipe rather than a filesystem socket.
func testListenAddr(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf(`\\.\pipe\dd-par-test-%d-%d`, os.Getpid(), testPipeCounter.Add(1))
}
