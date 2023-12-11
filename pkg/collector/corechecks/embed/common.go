// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(APL) Fix revive linter
package embed

import (
	"os/exec"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

const defaultRetryDuration = 5 * time.Second
const defaultRetries = 3

// retryExitError converts `exec.ExitError`s to `check.RetryableError`s, so that checks using this
// are retried.
// embed checks must use this from their `Run` method when exit errors need to be retried.
func retryExitError(err error) error { //nolint Used only on some architectures
	switch err.(type) {
	case *exec.ExitError: // error type returned when the process exits with non-zero status
		return check.RetryableError{Err: err}
	default:
		return err
	}
}
