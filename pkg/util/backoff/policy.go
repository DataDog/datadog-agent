// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backoff

import "time"

// Policy is the common interface for all backoff policies
type Policy interface {
	// GetBackoffDuration returns the backoff duration for the given number of errors
	GetBackoffDuration(numErrors int) time.Duration
	// IncError increments the number of errors and returns the new value
	IncError(numErrors int) int
	// DecError decrements the number of errors and returns the new value
	DecError(numErrors int) int
}
