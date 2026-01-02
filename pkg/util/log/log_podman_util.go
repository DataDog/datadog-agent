// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"strings"
)

// The paths below are set in podman code and cannot be modified by the user.
// Ref: https://github.com/containers/podman/blob/7c38ee756592d95e718967fcd3983b81abd95e76/test/e2e/run_transient_test.go#L19-L45
const (
	sqlDBSuffix  string = "/storage/db.sql"
	boltDBSuffix string = "/storage/libpod/bolt_state.db"
)

// ExtractPodmanRootDirFromDBPath extracts the podman base path for the containers directory based on the user-provided `podman_db_path`.
func ExtractPodmanRootDirFromDBPath(podmanDBPath string) string {
	if before, ok := strings.CutSuffix(podmanDBPath, sqlDBSuffix); ok {
		return before
	} else if before, ok := strings.CutSuffix(podmanDBPath, boltDBSuffix); ok {
		return before
	}
	return ""
}
