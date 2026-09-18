// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fakeintakeconfig

import (
	"github.com/DataDog/datadog-agent/test/fakeintake/version"
	"testing"
)

func TestImageURLUsesSharedPinAndOverride(t *testing.T) {
	if got := ImageURL("example.test/fixture", ""); got != "example.test/fixture:"+version.Tag {
		t.Fatal(got)
	}
	if got := ImageURL("example.test/fixture", "override.test/image@sha256:pin"); got != "override.test/image@sha256:pin" {
		t.Fatal(got)
	}
}
