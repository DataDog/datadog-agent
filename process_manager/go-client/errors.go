// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package procmgr

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// IsNotFound returns true if the error is a "not found" error
func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.NotFound
}

// IsAlreadyExists returns true if the error is an "already exists" error
func IsAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.AlreadyExists
}

// IsInvalidState returns true if the error is an "invalid state" error
func IsInvalidState(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.FailedPrecondition
}

// IsInvalidArgument returns true if the error is an "invalid argument" error
func IsInvalidArgument(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.InvalidArgument
}

// IsUnavailable returns true if the error is an "unavailable" error
func IsUnavailable(err error) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	return ok && st.Code() == codes.Unavailable
}
