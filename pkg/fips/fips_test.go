// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !goexperiment.boringcrypto

package fips

import (
	"crypto/fips140"
	"testing"
)

func TestEnabled(t *testing.T) {
	enabled, err := Enabled()
	if err != nil {
		t.Fatalf("Enabled() returned unexpected error: %v", err)
	}
	if enabled != fips140.Enabled() {
		t.Errorf("Enabled() = %v, want %v", enabled, fips140.Enabled())
	}
}

func TestStatus(t *testing.T) {
	status := Status()
	enabled, _ := Enabled()
	if enabled && status != "enabled" {
		t.Errorf("Status() = %q when Enabled() = true, want \"enabled\"", status)
	}
	if !enabled && status != "disabled" {
		t.Errorf("Status() = %q when Enabled() = false, want \"disabled\"", status)
	}
}
