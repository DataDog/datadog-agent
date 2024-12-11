// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pkgmanager contains pkgmanager implementations
package pkgmanager

// PackageManager generic interface
type PackageManager interface {
	Remove(pkg string) (string, error)
}
