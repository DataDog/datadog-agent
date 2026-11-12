// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package serializers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/schemas"
)

type fileCapabilitiesTestPayload struct {
	File    map[string]json.RawMessage `json:"file"`
	Process struct {
		Executable  map[string]json.RawMessage `json:"executable"`
		Credentials struct {
			Effective []string `json:"cap_effective"`
			Permitted []string `json:"cap_permitted"`
		} `json:"credentials"`
		Attempted []string `json:"caps_attempted"`
		Used      []string `json:"caps_used"`
		Source    string   `json:"source"`
	} `json:"process"`
	Capabilities *struct {
		Attempted []string `json:"caps_attempted"`
		Used      []string `json:"caps_used"`
	} `json:"capabilities"`
}

func newFileCapabilitiesTestEvent() *model.Event {
	// Initialize the capability name mappings used by serialization.
	model.SECLConstants()
	event := model.NewFakeEvent()
	event.Type = uint32(model.ExecEventType)
	proc := &event.ProcessContext.Process
	proc.Pid = 1234
	proc.Tid = 1234
	proc.Source = model.ProcessCacheEntryFromEvent
	proc.FileEvent.PathnameStr = "/usr/bin/test"
	proc.FileEvent.BasenameStr = "test"
	proc.FileEvent.Inode = 12345
	proc.FileEvent.MountID = 1
	proc.FileEvent.Mode = 0o755
	proc.FileEvent.Filesystem = "ext4"
	proc.FileEvent.MountSource = model.MountSourceMountID
	proc.FileEvent.MountOrigin = model.MountOriginEvent
	return event
}

func marshalFileCapabilitiesTestEvent(t *testing.T, event *model.Event) fileCapabilitiesTestPayload {
	t.Helper()
	data, err := MarshalEvent(event, nil, newTestScrubber(t))
	require.NoError(t, err)
	var payload fileCapabilitiesTestPayload
	require.NoError(t, json.Unmarshal(data, &payload))
	return payload
}

func TestFileCapabilitiesSerialization(t *testing.T) {
	caps := model.KernelCapabilityConstants
	fileSchema, err := gojsonschema.NewSchema(gojsonschema.NewReferenceLoaderFileSystem("file:///file.schema.json", http.FS(schemas.AssetFS)))
	require.NoError(t, err)

	for _, source := range []struct {
		name  string
		value uint64
	}{
		{"event", model.ProcessCacheEntryFromEvent},
		{"procfs_snapshot", model.ProcessCacheEntryFromSnapshot},
		{"procfs_fallback", model.ProcessCacheEntryFromProcFS},
	} {
		t.Run(source.name, func(t *testing.T) {
			for _, tc := range []struct {
				name        string
				caps        model.FileCapabilities
				permitted   []string
				inheritable []string
			}{
				{
					name: "distinct-low-and-high-masks",
					caps: model.FileCapabilities{
						Permitted:   caps["CAP_NET_BIND_SERVICE"] | caps["CAP_BPF"],
						Inheritable: caps["CAP_CHOWN"] | caps["CAP_AUDIT_READ"],
						Effective:   true,
						Available:   true,
					},
					permitted:   []string{"CAP_NET_BIND_SERVICE", "CAP_BPF"},
					inheritable: []string{"CAP_CHOWN", "CAP_AUDIT_READ"},
				},
				{
					name:      "permitted-only",
					caps:      model.FileCapabilities{Permitted: caps["CAP_NET_BIND_SERVICE"], Available: true},
					permitted: []string{"CAP_NET_BIND_SERVICE"},
				},
				{
					name:        "inheritable-only",
					caps:        model.FileCapabilities{Inheritable: caps["CAP_AUDIT_READ"], Available: true},
					inheritable: []string{"CAP_AUDIT_READ"},
				},
				{name: "empty-sets", caps: model.FileCapabilities{Available: true}},
				{name: "unavailable"},
				{
					name: "unavailable-with-stale-values",
					caps: model.FileCapabilities{
						Permitted:   caps["CAP_NET_BIND_SERVICE"],
						Inheritable: caps["CAP_AUDIT_READ"],
						Effective:   true,
					},
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					event := newFileCapabilitiesTestEvent()
					proc := &event.ProcessContext.Process
					proc.Source = source.value
					proc.FileCapabilities = tc.caps
					// Deliberately distinct from the file's declared capability sets.
					proc.CapEffective = caps["CAP_NET_RAW"]
					proc.CapPermitted = caps["CAP_NET_ADMIN"]
					proc.CapsAttempted = caps["CAP_SYS_ADMIN"]
					proc.CapsUsed = caps["CAP_SYS_PTRACE"]

					payload := marshalFileCapabilitiesTestEvent(t, event)
					assert.Equal(t, source.name, payload.Process.Source)
					assert.Equal(t, []string{"CAP_NET_RAW"}, payload.Process.Credentials.Effective)
					assert.Equal(t, []string{"CAP_NET_ADMIN"}, payload.Process.Credentials.Permitted)
					assert.Equal(t, []string{"CAP_SYS_ADMIN"}, payload.Process.Attempted)
					assert.Equal(t, []string{"CAP_SYS_PTRACE"}, payload.Process.Used)
					assert.Nil(t, payload.Capabilities, "exec metadata is not a capability-usage event")

					for _, location := range []struct {
						name string
						file map[string]json.RawMessage
					}{
						{"file", payload.File},
						{"process.executable", payload.Process.Executable},
					} {
						t.Run(location.name, func(t *testing.T) {
							require.NotNil(t, location.file)
							result, err := fileSchema.Validate(gojsonschema.NewGoLoader(location.file))
							require.NoError(t, err)
							assert.True(t, result.Valid(), "file schema errors: %v", result.Errors())
							if !tc.caps.Available {
								assert.NotContains(t, location.file, "capabilities", "unavailable metadata must be omitted, not serialized as null or empty sets")
								return
							}

							require.Contains(t, location.file, "capabilities")
							var got struct {
								Permitted   []string `json:"permitted"`
								Inheritable []string `json:"inheritable"`
								Effective   bool     `json:"effective"`
								Available   bool     `json:"available"`
							}
							require.NoError(t, json.Unmarshal(location.file["capabilities"], &got))
							require.NotNil(t, got.Permitted, "empty permitted sets must be [], not null")
							require.NotNil(t, got.Inheritable, "empty inheritable sets must be [], not null")
							assert.ElementsMatch(t, tc.permitted, got.Permitted)
							assert.ElementsMatch(t, tc.inheritable, got.Inheritable)
							assert.Equal(t, tc.caps.Effective, got.Effective)
							assert.True(t, got.Available)
						})
					}
				})
			}
		})
	}
}

func TestFileCapabilitiesDoNotImplyCredentialsOrUsage(t *testing.T) {
	event := newFileCapabilitiesTestEvent()
	event.ProcessContext.FileCapabilities = model.FileCapabilities{
		Permitted: model.KernelCapabilityConstants["CAP_NET_BIND_SERVICE"],
		Effective: true,
		Available: true,
	}
	payload := marshalFileCapabilitiesTestEvent(t, event)
	require.Contains(t, payload.File, "capabilities")
	require.Contains(t, payload.Process.Executable, "capabilities")
	assert.Empty(t, payload.Process.Credentials.Effective)
	assert.Empty(t, payload.Process.Credentials.Permitted)
	assert.Empty(t, payload.Process.Attempted)
	assert.Empty(t, payload.Process.Used)
	assert.Nil(t, payload.Capabilities)
}

func TestFileCapabilitiesSeparateFromCapabilityUsageEvent(t *testing.T) {
	event := newFileCapabilitiesTestEvent()
	caps := model.KernelCapabilityConstants
	event.Type = uint32(model.CapabilitiesEventType)
	event.ProcessContext.FileCapabilities = model.FileCapabilities{
		Permitted: caps["CAP_NET_BIND_SERVICE"], Effective: true, Available: true,
	}
	event.CapabilitiesUsage.Attempted = caps["CAP_SETFCAP"]
	event.CapabilitiesUsage.Used = caps["CAP_SYS_PTRACE"]

	payload := marshalFileCapabilitiesTestEvent(t, event)
	require.Contains(t, payload.Process.Executable, "capabilities")
	require.NotNil(t, payload.Capabilities)
	assert.Equal(t, []string{"CAP_SETFCAP"}, payload.Capabilities.Attempted)
	assert.Equal(t, []string{"CAP_SYS_PTRACE"}, payload.Capabilities.Used)
	assert.Empty(t, payload.Process.Credentials.Effective)
	assert.Empty(t, payload.Process.Credentials.Permitted)
	assert.Empty(t, payload.Process.Used)
}
