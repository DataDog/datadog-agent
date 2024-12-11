// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package checks

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
)

// NewProcessDataWithMockProbe returns a new ProcessData with a mock probe
func NewProcessDataWithMockProbe(t *testing.T) (*ProcessData, *mocks.Probe) {
	probe := mocks.NewProbe(t)
	return &ProcessData{
		probe: probe,
	}, probe
}
