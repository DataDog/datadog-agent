// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"fmt"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
	"time"
)

// RequireAssertions provide some additional assertions for
// the BaseSuite based on require.Assertions
type RequireAssertions struct {
	*require.Assertions
}

// EventuallyWithExponentialBackoff replaces EventuallyWithT with synchronous exponential backoff
func (r *RequireAssertions) EventuallyWithExponentialBackoff(condition func() error, maxElapsedTime, maxInterval time.Duration, msgAndArgs ...interface{}) {
	err := backoff.Retry(condition, backoff.NewExponentialBackOff(
		backoff.WithMaxInterval(maxInterval),
		backoff.WithMaxElapsedTime(maxElapsedTime),
	))
	if err != nil {
		r.Fail(fmt.Sprintf("Condition never satisfied: %v", err), msgAndArgs...)
	}
}
