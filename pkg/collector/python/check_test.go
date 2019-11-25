// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build python,test

package python

import (
	"testing"
)

func TestRunCheck(t *testing.T) {
	testRunCheck(t)
}

func TestRunErrorNil(t *testing.T) {
	testRunErrorNil(t)
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
