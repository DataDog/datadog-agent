// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package procnetsocket

import (
	"context"
	"errors"
)

// All read* functions are non-Linux stubs; /proc/net/ socket reading is Linux-only.

func readTCP4(_ context.Context, _ string) ([]SocketEntry, error) {
	return nil, errors.New("socket reading is not supported on this platform")
}

func readTCP6(_ context.Context, _ string) ([]SocketEntry, error) {
	return nil, errors.New("socket reading is not supported on this platform")
}

func readUDP4(_ context.Context, _ string) ([]SocketEntry, error) {
	return nil, errors.New("socket reading is not supported on this platform")
}

func readUDP6(_ context.Context, _ string) ([]SocketEntry, error) {
	return nil, errors.New("socket reading is not supported on this platform")
}

func readUnix(_ context.Context, _ string) ([]SocketEntry, error) {
	return nil, errors.New("socket reading is not supported on this platform")
}
