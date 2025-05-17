// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

// Package testutil contains test utilities for the eBPF uprobe package.
package testutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
)

var mux sync.Mutex

// BuildStandaloneAttacher builds the standalone attacher binary and returns the path to the binary
func BuildStandaloneAttacher(t *testing.T) string {
	mux.Lock()
	defer mux.Unlock()

	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	attacherBin, err := usmtestutil.BuildGoBinaryWrapper(curDir, "standalone_attacher")
	require.NoError(t, err)
	return attacherBin
}
