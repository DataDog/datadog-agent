// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packagemanager

import (
	"context"
)

func RemovePackage(_ context.Context, _ string) (err error) {
	return nil // Noop on Windows
}
