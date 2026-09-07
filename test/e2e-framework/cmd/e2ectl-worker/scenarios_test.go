// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "testing"

func TestEC2FakeintakeSelection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra string
		want  bool
	}{
		{name: "framework default", want: true},
		{name: "enabled", extra: "fakeintake: true\n", want: true},
		{name: "disabled", extra: "fakeintake: false\n", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := decodeEC2Params("os: ubuntu-22.04\narch: amd64\n" + tc.extra)
			if err != nil {
				t.Fatal(err)
			}
			if p.fakeintakeEnabled() != tc.want {
				t.Fatalf("fakeintake enabled = %v, want %v", p.fakeintakeEnabled(), tc.want)
			}
		})
	}
}

func TestEC2ParamsRejectInvalidFakeintake(t *testing.T) {
	for _, extra := range []string{"fakeintake: not-a-bool", "fakeintak: false"} {
		if _, err := decodeEC2Params("os: ubuntu-22.04\narch: amd64\n" + extra); err == nil {
			t.Fatalf("expected an error for %q", extra)
		}
	}
}
