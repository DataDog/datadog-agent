// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !darwin

package fdhandoff

import (
	"os"
	"time"
)

// DefaultWaitTimeout is unused on platforms without SCM_RIGHTS, where the
// handoff is never attempted, but keeps the package API platform-independent.
func DefaultWaitTimeout() time.Duration { return 10 * time.Second }

// receiveFileWithin returns a "not implemented" error on platforms without SCM_RIGHTS.
func receiveFileWithin(_ string, _ time.Duration) (*os.File, error) {
	return nil, ErrUnsupported
}
