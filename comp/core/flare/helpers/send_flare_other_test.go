// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !darwin

package helpers

import "testing"

func assertArchiveClosed(t *testing.T, _ string) {
	t.Helper()
	// assertUploadOver covers the close: the upload goroutine closes the archive before it signals done.
	t.Log("no open-descriptor probe on this platform")
}
