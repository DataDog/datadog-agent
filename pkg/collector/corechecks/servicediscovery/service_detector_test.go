// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_serviceDetector(t *testing.T) {
	sd := newServiceDetector()

	// no need to test many cases here, just ensuring the process data is properly passed down is enough.
	pInfo := processInfo{
		PID:     100,
		CmdLine: []string{"my-service.py"},
		Env:     []string{"PATH=testdata/test-bin", "DD_INJECTION_ENABLED=tracer"},
		Cwd:     "",
		Stat:    procStat{},
		Ports:   []int{5432},
	}

	want := serviceMetadata{
		Name:               "my-service",
		Language:           "python",
		Type:               "db",
		APMInstrumentation: "injected",
	}
	got := sd.Detect(pInfo)
	assert.Equal(t, want, got)
}
