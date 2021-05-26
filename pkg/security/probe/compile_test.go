// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux_bpf

package probe

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/stretchr/testify/require"
)

func TestProbeCompile(t *testing.T) {
	cfg := ebpf.NewConfig()
	var cflags []string
	_, err := runtime.RuntimeSecurity.Compile(cfg, cflags)
	require.NoError(t, err)
}
