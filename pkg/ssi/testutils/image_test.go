// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
)

func TestImageValidator(t *testing.T) {
	tests := map[string]struct {
		in      string
		require func(t *testing.T, v *testutils.ImageValidator)
	}{
		"ensure registry matches expected": {
			in: "gcr.io/datadoghq/dd-lib-java-init:v1",
			require: func(t *testing.T, v *testutils.ImageValidator) {
				v.RequireRegistry(t, "gcr.io/datadoghq")
			},
		},
		"ensure image matches expected": {
			in: "gcr.io/datadoghq/dd-lib-java-init:v1",
			require: func(t *testing.T, v *testutils.ImageValidator) {
				v.RequireName(t, "dd-lib-java-init")
			},
		},
		"ensure tag matches expected": {
			in: "gcr.io/datadoghq/dd-lib-java-init:v1",
			require: func(t *testing.T, v *testutils.ImageValidator) {
				v.RequireTag(t, "v1")
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := testutils.NewImageValidator(test.in)
			test.require(t, v)
		})
	}
}
