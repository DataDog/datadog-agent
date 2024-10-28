// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpf holds ebpf related files
package ebpf

import (
	"testing"

	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
)

func TestLoaderCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg, err := config.NewConfig()
		require.NoError(t, err)
		out, err := getRuntimeCompiledPrograms(cfg, false, false, false, nil)
		require.NoError(t, err)
		_ = out.Close()
	})
}
