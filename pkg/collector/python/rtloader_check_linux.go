// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && linux

package python

import (
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// checkRtloaderAvailable uses dlopen to verify that the rtloader shared library
// is present on the system before attempting to initialize it. On Linux the
// rtloader is a shared library loaded at runtime, so a missing library produces
// cryptic dlopen errors. This check provides a clear, actionable message.
func checkRtloaderAvailable() error {
	return system.CheckLibraryExists(rtloaderLibraryName)
}
