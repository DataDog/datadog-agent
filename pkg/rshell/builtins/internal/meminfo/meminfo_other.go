// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package meminfo

import "context"

// readImpl returns ErrNotSupported on platforms without a backend
// (currently everything but Linux — see the package doc comment for why
// macOS and Windows are not implemented).
func readImpl(_ context.Context) (Info, error) {
	return Info{}, ErrNotSupported
}
