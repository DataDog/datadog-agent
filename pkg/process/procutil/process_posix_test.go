// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || freebsd || openbsd || darwin

package procutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPidExists(t *testing.T) {
	require.True(t, PidExists(os.Getpid()))
	require.False(t, PidExists(0xdeadbeef))
}
