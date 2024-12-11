// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// RetryableError is the error type that Retry interprets as a request to retry the callback
type RetryableError struct {
	Err error
}

func (e RetryableError) Error() string {
	return e.Err.Error()
}

// Retry retries a callback up to `retries`-times in a `retryDuration` period of time, and bails out
// if the limit is reached.
func Retry(retryDuration time.Duration, retries int, callback func() error, friendlyName string) (err error) {
	attempts := 0

	for {
		t0 := time.Now()
		err = callback()
		if err == nil {
			return nil
		}
		switch err.(type) {
		case RetryableError:
			// proceed with retry
		default:
			// No retry requested, return the error
			return err
		}

		// how much did the callback run?
		execDuration := time.Since(t0)
		if execDuration < retryDuration {
			// the callback failed too soon, retry but increment the counter
			attempts++
		} else {
			// the callback failed after the retryDuration, reset the counter
			attempts = 0
		}

		if attempts == retries {
			// give up
			return fmt.Errorf("bail out from %s, max retries reached, last error: %v", friendlyName, err)
		}

		log.Warnf("retrying %s, got the error: %v", friendlyName, err)
	}
}
