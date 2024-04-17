// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package service provides a way to interact with os services
package service

import (
	"context"
	"os"
)

// ChownDDAgent changes the owner of the given path to the dd-agent user.
func ChownDDAgent(_ context.Context, _ string) error {
	return nil
}

// RemoveAll removes the versioned files at a given path.
func RemoveAll(_ context.Context, path string) error {
	return os.RemoveAll(path)
}

// BuildHelperForTests builds the helper binary for test
func BuildHelperForTests(_, _ string, _ bool) error {
	return nil
}

// SetCapHelper sets cap setuid on the newly installed helper
func SetCapHelper(_ context.Context, _ string) error {
	return nil
}
