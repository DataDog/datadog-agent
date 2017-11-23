// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package fanout

import (
	"errors"
)

var (
	// ErrWriteTimeout is sent when a suscriber falls behind (its channel is full)
	// and is forcefully unsuscribed.
	ErrWriteTimeout = errors.New("timeout while writing to channel")
)

// IsWriteTimeout checks whether an error is a ErrWriteTimeout
func IsWriteTimeout(err error) bool {
	return err.Error() == ErrWriteTimeout.Error()
}
