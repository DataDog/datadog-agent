// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCORETelemetry(t *testing.T) {
	storeCORETelemetryForAsset("exampleAsset1", COREResult(successCustomBTF))
	storeCORETelemetryForAsset("exampleAsset2", verifierError)

	actual := GetCORETelemetryByAsset()
	expected := map[string]int32{
		"exampleAsset1": int32(successCustomBTF),
		"exampleAsset2": int32(verifierError),
	}

	assert.Equal(t, expected, actual)
}
