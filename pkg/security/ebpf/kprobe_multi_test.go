// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"
	"os"
	"strings"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

func TestKprobeMultiBackendPreservesPrograms(t *testing.T) {
	specs := map[string]*lib.ProgramSpec{
		"entry":      {Type: lib.Kprobe, SectionName: "kprobe/func", AttachTo: "func"},
		"return":     {Type: lib.Kprobe, SectionName: "kretprobe/func", AttachTo: "func"},
		"tail_call":  {Type: lib.Kprobe, SectionName: "kprobe/tail_call_target", AttachTo: "tail_call_target"},
		"uprobe":     {Type: lib.Kprobe, SectionName: "uprobe/func", AttachTo: "func"},
		"uretprobe":  {Type: lib.Kprobe, SectionName: "uretprobe/func", AttachTo: "func"},
		"tracepoint": {Type: lib.TracePoint, SectionName: "tracepoint/syscalls/sys_enter_open"},
		"fentry":     {Type: lib.Tracing, AttachType: lib.AttachTraceFEntry, SectionName: "fentry/func", AttachTo: "func"},
		"fexit":      {Type: lib.Tracing, AttachType: lib.AttachTraceFExit, SectionName: "fexit/func", AttachTo: "func"},
	}
	expected := make(map[string]*lib.ProgramSpec, len(specs))
	for name, spec := range specs {
		spec.Name = name
		spec.Instructions = asm.Instructions{asm.Mov.Imm(asm.R0, 0), asm.Return()}
		expected[name] = spec.Copy()
	}
	for _, name := range []string{"entry", "return", "tail_call"} {
		expected[name].AttachType = lib.AttachTraceKprobeMulti
	}
	retargetKprobePrograms(specs)
	require.Equal(t, expected, specs, "only the expected attach type of kprobe callers and targets may change")
}

func TestKprobeMultiBackendDisabled(t *testing.T) {
	t.Setenv("DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_KPROBE_MULTI", "")
	for _, value := range []string{"", "false", "grouped", "grouped-return"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", value)
			configmock.NewSystemProbe(t)
			// A disabled backend must neither query the kernel nor access the manager.
			backend := &kprobeMultiBackend{checkSupport: func() error {
				t.Fatal("disabled backend must not check support")
				return nil
			}}
			require.NoError(t, backend.BeforeInit(nil, names.ModuleName{}, nil))
		})
	}
}

func TestKprobeMultiBackendFallsBackBeforeRetargeting(t *testing.T) {
	t.Setenv("DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_KPROBE_MULTI", "")
	for name, supportErr := range map[string]error{
		"unsupported_kernel": lib.ErrNotSupported,
		"permission_denied":  os.ErrPermission,
		"probe_failure":      errors.New("unexpected capability probe failure"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", "true")
			configmock.NewSystemProbe(t)
			checked := false
			backend := &kprobeMultiBackend{checkSupport: func() error {
				checked = true
				return supportErr
			}}
			// A nil manager proves fallback happens before accessing or changing
			// any program specs, keeping callers and tail-call targets on legacy.
			require.NoError(t, backend.BeforeInit(nil, names.ModuleName{}, nil))
			require.True(t, checked)
		})
	}
}

func TestKprobeMultiBackendPreservesManagerErrors(t *testing.T) {
	t.Setenv("DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_KPROBE_MULTI", "")
	t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", "true")
	configmock.NewSystemProbe(t)
	backend := &kprobeMultiBackend{checkSupport: func() error { return nil }}
	m := &manager.Manager{}
	_, expectedErr := m.GetProgramSpecs()
	require.Error(t, expectedErr)
	// Capability fallback must not hide unrelated initialization failures.
	require.ErrorIs(t, backend.BeforeInit(m, names.ModuleName{}, nil), expectedErr)
}

func TestKprobeMultiBackendConfig(t *testing.T) {
	for _, tt := range []struct {
		name      string
		yamlValue string
		envValue  string
		legacyEnv string
		wantCheck bool
	}{
		{name: "default"},
		{name: "yaml_enabled", yamlValue: "true", wantCheck: true},
		{name: "yaml_disabled", yamlValue: "false"},
		{name: "env_enabled", envValue: "true", wantCheck: true},
		{name: "legacy_env_enabled", legacyEnv: "true", wantCheck: true},
		{name: "env_overrides_yaml_enabled", yamlValue: "true", envValue: "false"},
		{name: "env_overrides_yaml_disabled", yamlValue: "false", envValue: "true", wantCheck: true},
		{name: "legacy_env_overrides_yaml", yamlValue: "true", legacyEnv: "false"},
		{name: "canonical_env_precedes_legacy", envValue: "false", legacyEnv: "true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_EVENT_MONITORING_CONFIG_EVENT_STREAM_USE_KPROBE_MULTI", tt.envValue)
			t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", tt.legacyEnv)
			cfg := configmock.NewSystemProbe(t)
			if tt.yamlValue != "" {
				cfg.SetConfigType("yaml")
				require.NoError(t, cfg.ReadConfig(strings.NewReader("event_monitoring_config:\n  event_stream:\n    use_kprobe_multi: "+tt.yamlValue)))
			}
			checked := false
			backend := &kprobeMultiBackend{checkSupport: func() error {
				checked = true
				return lib.ErrNotSupported
			}}
			require.NoError(t, backend.BeforeInit(nil, names.ModuleName{}, nil))
			require.Equal(t, tt.wantCheck, checked)
		})
	}
}
