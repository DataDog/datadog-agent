// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fixtures

import (
	"encoding/json"
	"testing"
)

func TestUserDefaultAndNormalizedTransport(t *testing.T) {
	defaulted, _, err := Schema.Decode(nil, "environment")
	if err != nil || !defaulted.FakeIntake {
		t.Fatalf("user omission should apply the schema default: %+v %v", defaulted, err)
	}
	for _, enabled := range []bool{false, true} {
		data, err := json.Marshal(Config{FakeIntake: enabled})
		if err != nil {
			t.Fatal(err)
		}
		var got Config
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatal(err)
		}
		if got.FakeIntake != enabled {
			t.Fatal("transport changed the explicit value")
		}
	}
}

func TestNormalizedTransportRejectsMissingAndNull(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `{"fakeintake":null}`, `{"fakeintake":"false"}`, `{"fakeintake":false,"unknown":true}`, `{"fakeintake":true,"fakeintake":false}`} {
		t.Run(raw, func(t *testing.T) {
			var got Config
			if err := json.Unmarshal([]byte(raw), &got); err == nil {
				t.Fatal("invalid wire boolean became a Go zero value")
			}
		})
	}
}
