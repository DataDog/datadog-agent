// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
)

func TestGetSubprocessOutputEmptyArgs(t *testing.T) {
	testGetSubprocessOutputEmptyArgs(t)
}

func TestGetSubprocessOutput(t *testing.T) {
	testGetSubprocessOutput(t)
}

func TestGetSubprocessOutputUnknownBin(t *testing.T) {
	testGetSubprocessOutputUnknownBin(t)
}

func TestGetSubprocessOutputError(t *testing.T) {
	testGetSubprocessOutputError(t)
}

func TestGetSubprocessOutputEnv(t *testing.T) {
	testGetSubprocessOutputEnv(t)
}
