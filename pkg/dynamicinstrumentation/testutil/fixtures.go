// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package testutil

import (
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
)

// TestInstrumentationOptions contains options for probes in tests
type TestInstrumentationOptions struct {
	CaptureDepth int
}

// CapturedValueMapWithOptions pairs instrumentaiton options with expected values
type CapturedValueMapWithOptions struct {
	CapturedValueMap ditypes.CapturedValueMap
	Options          TestInstrumentationOptions
}
