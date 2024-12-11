// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package filesystem provides functions and types to interact with the filesystem
package filesystem

// DiskUsage is the disk usage
type DiskUsage struct {
	Total     uint64
	Available uint64
}
