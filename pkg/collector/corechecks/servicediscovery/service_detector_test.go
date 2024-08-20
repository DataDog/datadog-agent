// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package servicediscovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_serviceDetector(t *testing.T) {
	sd := NewServiceDetector()

	// no need to test many cases here, just ensuring the process data is properly passed down is enough.
	pInfo := processInfo{
		PID:     100,
		CmdLine: []string{"my-service.py"},
		Env:     map[string]string{"PATH": "testdata/test-bin", "DD_INJECTION_ENABLED": "tracer"},
		Stat:    procStat{},
		Ports:   []uint16{5432},
	}

	want := ServiceMetadata{
		Name:               "my-service",
		Language:           "python",
		Type:               "db",
		APMInstrumentation: "injected",
		NameSource:         "generated",
	}
	got := sd.Detect(pInfo)
	assert.Equal(t, want, got)

	// pass in nil slices and see if anything blows up
	pInfoEmpty := processInfo{
		PID:     0,
		CmdLine: nil,
		Env:     nil,
		Stat:    procStat{},
		Ports:   nil,
	}
	wantEmpty := ServiceMetadata{
		Name:               "",
		Language:           "UNKNOWN",
		Type:               "web_service",
		APMInstrumentation: "none",
		NameSource:         "generated",
	}
	gotEmpty := sd.Detect(pInfoEmpty)
	assert.Equal(t, wantEmpty, gotEmpty)
}
