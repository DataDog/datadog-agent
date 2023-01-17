// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package model

import (
	"fmt"
	"syscall"
	"testing"
)

func TestFlagsToString(t *testing.T) {
	str := OpenFlags(syscall.O_EXCL | syscall.O_TRUNC).String()
	if str != "O_EXCL | O_TRUNC" {
		t.Errorf("expected flags not found, got: %s", str)
	}

	str = ChmodMode(syscall.S_IWGRP | syscall.S_IRUSR).String()
	if str != "S_IRUSR | S_IWGRP" {
		t.Errorf("expected flags not found, got: %s", str)
	}

	str = OpenFlags(syscall.O_EXCL | syscall.O_TRUNC | 1<<32).String()
	if str != fmt.Sprintf("%d | O_EXCL | O_TRUNC", 1<<32) {
		t.Errorf("expected flags not found, got: %s", str)
	}

	str = OpenFlags(syscall.O_RDONLY).String()
	if str != "O_RDONLY" {
		t.Errorf("expected flags not found, got: %s", str)
	}
}
