// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python && test

package python

import (
	"testing"
)

func TestRunCheck(t *testing.T) {
	testRunCheck(t)
}

func TestInitCheckWithRuntimeNotInitialized(t *testing.T) {
	testInitiCheckWithRuntimeNotInitialized(t)
}

func TestRunCheckWithRuntimeNotInitialized(t *testing.T) {
	testRunCheckWithRuntimeNotInitializedError(t)
}

func TestRunErrorNil(t *testing.T) {
	testRunErrorNil(t)
}

func TestCheckCancel(t *testing.T) {
	testCheckCancel(t)
}

func TestCheckCancelWhenRuntimeUnloaded(t *testing.T) {
	testCheckCancelWhenRuntimeUnloaded(t)
}

func TestFinalizer(t *testing.T) {
	testFinalizer(t)
}

func TestFinalizerWhenRuntimeUnloaded(t *testing.T) {
	testFinalizerWhenRuntimeUnloaded(t)
}

func TestRunErrorReturn(t *testing.T) {
	testRunErrorReturn(t)
}

func TestRun(t *testing.T) {
	testRun(t)
}

func TestRunSimple(t *testing.T) {
	testRunSimple(t)
}

func TestConfigure(t *testing.T) {
	testConfigure(t)
}

func TestConfigureDeprecated(t *testing.T) {
	testConfigureDeprecated(t)
}
