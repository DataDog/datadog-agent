// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package file

// staleFileHandleHook overrides platform ESTALE detection in tests when set.
var staleFileHandleHook func(error) bool

// IsStaleFileHandle reports whether err is a stale file handle (ESTALE on Linux).
func IsStaleFileHandle(err error) bool {
	if staleFileHandleHook != nil {
		return staleFileHandleHook(err)
	}
	return isStaleFileHandlePlatform(err)
}
