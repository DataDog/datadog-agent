// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

// Package testutil contains test utilities for the CUDA package.
package testutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
)

// SamplesSMVersionSet returns a set of all the SM versions present in the sample files.
func SamplesSMVersionSet() map[uint32]struct{} {
	smSet := make(map[uint32]struct{})
	for _, sm := range SampleSMVersions {
		smSet[sm] = struct{}{}
	}
	return smSet
}

// SampleSMVersions has all the SM version of the kernels.
// Retrieved from the Makefile, all the -gencode arch=compute_XX,code=sm_XX flags
var SampleSMVersions = []uint32{50, 52, 60, 61, 70, 75, 80, 86, 89, 90}

// GetCudaSample returns the path to the CUDA fatbin file for the given name.
// The test data is a CUDA fatbin file compiled with the Makefile present in the same directory,
// using `make <name>` (for now, only supported samples are `sample` and `heavy-sample`).
func GetCudaSample(t testing.TB, name string) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	sample := filepath.Join(curDir, "testdata", name)
	require.FileExists(t, sample)

	return sample
}
