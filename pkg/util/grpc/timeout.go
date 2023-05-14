// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DoWithTimeout runs f and returns its error.  If the deadline d elapses first,
// it returns a grpc DeadlineExceeded error instead.
func DoWithTimeout(f func() error, d time.Duration) error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- f()
		close(errChan)
	}()

	t := time.NewTimer(d)
	select {
	case <-t.C:
		return status.Errorf(codes.DeadlineExceeded, "timed out waiting for operation")
	case err := <-errChan:
		if !t.Stop() {
			<-t.C
		}
		return err
	}
}
