// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux || !cgo || static

package system

import "errors"

// CheckLibraryExists checks if a library is available on the system by trying it to
// open with dlopen. It returns an error if the library is not found.
func CheckLibraryExists(_ string) error {
	return errors.New("CheckLibrary is not supported on this platform")
}
