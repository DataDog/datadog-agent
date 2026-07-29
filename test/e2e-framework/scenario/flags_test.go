// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestRegisterAndCollectFlags(t *testing.T) {
	s, _ := BuildSchema(&schemaSample{})
	fs := pflag.NewFlagSet("t", pflag.ContinueOnError)
	RegisterFlags(s, fs)

	if err := fs.Parse([]string{"--token", "abc", "--verbose", "--replicas", "5"}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	got := CollectFlags(s, fs)
	if got["token"] != "abc" || got["verbose"] != "true" || got["replicas"] != "5" {
		t.Fatalf("collected wrong: %v", got)
	}
	// os was not set on the command line -> must not appear (Decode applies the default).
	if _, ok := got["os"]; ok {
		t.Errorf("unchanged flag os should not be collected")
	}
}
