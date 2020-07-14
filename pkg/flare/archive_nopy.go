// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !python

package flare

func writePyHeapProfile(tempDir, hostname string) error {
	return nil
}

func rtLoaderEnabled() bool {
	return false
}
