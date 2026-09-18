// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "testing"

func TestReceiverSelectionEnvelope(t *testing.T) {
	for _, tc := range []struct {
		section string
		valid   bool
	}{
		{"    type: blackhole\n    blackhole: {url: 'http://sink:8080'}\n", true},
		{"    blackhole: {url: 'http://sink:8080'}\n    type: blackhole\n", true},
		{"    type: fakeintake\n    datadog: {}\n", false},
		{"    type: fakeintake\n    type: blackhole\n", false},
		{"    fakeintake: {}\n", false},
	} {
		f, errs := Parse([]byte("schema: 1\nenvironment: {base: local}\nagent:\n  receiver:\n" + tc.section + "  install: binary\n"))
		if (len(errs) == 0) != tc.valid {
			t.Fatalf("%s: %v", tc.section, errs)
		}
		if tc.valid && f.Agent.Receiver.Type != "blackhole" {
			t.Fatal("selector lost")
		}
	}
}
