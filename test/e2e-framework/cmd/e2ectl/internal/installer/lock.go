// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/e2ectl/internal/envstore"
	"os"
	"path/filepath"
)

// LockAgentOperation serializes Agent mutation with local runtime teardown.
// Callers must release it even on failure; stale locks require manual inspection.
func LockAgentOperation(entry envstore.Entry) (func(), error) {
	path := filepath.Join(entry.Dir, ".agent-operation.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, fmt.Errorf("Agent operation locked (%s); inspect any running operation before removing a stale lock: %w", path, err)
	}
	f.Close()
	return func() { os.Remove(path) }, nil
}
