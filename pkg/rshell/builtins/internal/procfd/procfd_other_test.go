// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package procfd

import (
	"errors"
	"testing"
)

func TestListNotSupportedOffLinux(t *testing.T) {
	_, err := List(t.Context(), "/proc", []int{1}, nil)
	if !errors.Is(err, ErrNotSupported) {
		t.Errorf("List = %v, want ErrNotSupported", err)
	}
}
