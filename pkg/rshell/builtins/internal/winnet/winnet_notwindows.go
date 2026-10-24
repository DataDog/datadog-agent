// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package winnet

import "errors"

// Collect returns an error on non-Windows platforms; ss_windows.go is only
// compiled on Windows so this stub is never called in practice.
func Collect() ([]SocketEntry, error) {
	return nil, errors.New("not implemented on this platform")
}
