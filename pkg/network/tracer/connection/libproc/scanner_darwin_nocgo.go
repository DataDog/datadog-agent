// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && !cgo

package libproc

import "errors"

// NativeScanner is unavailable when cgo is disabled.
type NativeScanner struct{}

// NewNativeScanner reports that Darwin libproc requires cgo.
func NewNativeScanner(Limits) (*NativeScanner, error) {
	return nil, errors.New("Darwin libproc scanner requires cgo")
}

// Scan reports that Darwin libproc requires cgo.
func (*NativeScanner) Scan() (Snapshot, error) {
	return Snapshot{}, errors.New("Darwin libproc scanner requires cgo")
}
