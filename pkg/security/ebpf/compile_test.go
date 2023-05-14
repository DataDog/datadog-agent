// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

func TestLoaderCompile(t *testing.T) {
	syscfg, err := sysconfig.New("")
	require.NoError(t, err)
	cfg, err := config.NewConfig(syscfg)
	require.NoError(t, err)
	_, err = getRuntimeCompiledPrograms(cfg, false, false, nil)
	require.NoError(t, err)
}
