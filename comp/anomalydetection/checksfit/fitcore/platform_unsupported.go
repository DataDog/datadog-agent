// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && (!darwin || !cgo)

package fitcore

import "errors"

// errUnsupported reports that the FIT transport has no implementation for the
// current platform. The supported profile is little-endian amd64/arm64 Linux
// and macOS 14.4 or newer with cgo enabled. Callers can detect this build with
// the checksfit package's Supported constant and check errors against
// ErrUnsupported.
var errUnsupported = errors.New("FIT is only supported on little-endian amd64/arm64 Linux and macOS with cgo enabled")

// ErrUnsupported reports that this build has no FIT transport. Callers that
// need a portable capability check should use checksfit.Supported instead.
var ErrUnsupported = errUnsupported

// checkOSVersion reports that this platform has no FIT implementation.
func checkOSVersion() error { return errUnsupported }

// shmOpen reports that this platform has no FIT implementation.
func shmOpen(string, int, uint32) (int, error) { return -1, errUnsupported }

// shmUnlink reports that this platform has no FIT implementation.
func shmUnlink(string) error { return errUnsupported }

// shmExpectedBackingSize reports that this platform has no FIT implementation.
func shmExpectedBackingSize(int) (int64, error) { return 0, errUnsupported }

// shmModeAllowed reports that this platform has no FIT implementation.
func shmModeAllowed(uint32) bool { return false }

// waitWord reports that this platform has no FIT implementation.
func waitWord(*uint32, uint32) error { return errUnsupported }

// wakeWord reports that this platform has no FIT implementation.
func wakeWord(*uint32) error    { return errUnsupported }
func wakeAllWord(*uint32) error { return errUnsupported }
