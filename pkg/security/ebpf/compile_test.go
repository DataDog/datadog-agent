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

	"github.com/DataDog/datadog-agent/pkg/ebpf"
)

func TestLoaderCompile(t *testing.T) {
	cfg := ebpf.NewConfig()
	_, err := getRuntimeCompiledProbe(cfg, false, nil)
	require.NoError(t, err)
}
