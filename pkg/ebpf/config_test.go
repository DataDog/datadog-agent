// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	aconfig "github.com/DataDog/datadog-agent/pkg/config"
)

func TestEnableEBPFInstrumentation(t *testing.T) {
	t.Run("via YAML", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		_, err := sysconfig.New("./testdata/TestDDAgentConfigYamlAndSystemProbeConfig-EnableEBPFInstrumentation.yaml")
		require.NoError(t, err)
		cfg := NewConfig()

		assert.True(t, cfg.EBPFInstrumentationEnabled)
	})

	t.Run("via ENV variable", func(t *testing.T) {
		aconfig.ResetSystemProbeConfig(t)
		t.Setenv("DD_ENABLE_EBPF_INSTRUMENTATION", "true")
		_, err := sysconfig.New("")
		require.NoError(t, err)
		cfg := NewConfig()

		assert.True(t, cfg.EBPFInstrumentationEnabled)
	})
}
