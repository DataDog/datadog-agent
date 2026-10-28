// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

// Package ebpf holds ebpf related files
package ebpf

import (
	"testing"

	"github.com/stretchr/testify/require"

	schemanoopimpl "github.com/DataDog/datadog-agent/comp/core/schema/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

func TestLoaderCompile(t *testing.T) {
	ebpftest.TestBuildMode(t, ebpftest.RuntimeCompiled, "", func(t *testing.T) {
		_, err := sysconfig.New("", "", &schemanoopimpl.SchemaNoop{})
		require.NoError(t, err)
		cfg, err := config.NewConfig()
		require.NoError(t, err)
		out, err := getRuntimeCompiledPrograms(cfg, false, false, false, false)
		require.NoError(t, err)
		_ = out.Close()
	})
}
