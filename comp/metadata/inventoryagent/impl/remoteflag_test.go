// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventoryagentimpl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/metadata/internal/agentmetadata"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgremoteflags "github.com/DataDog/datadog-agent/pkg/remoteflags"
)

type fakeSettings struct {
	value  bool
	source model.Source
}

func (f *fakeSettings) GetBool(string) bool           { return f.value }
func (f *fakeSettings) GetSource(string) model.Source { return f.source }

func TestFullConfigFlagName(t *testing.T) {
	h := newFullConfigFlagHandler(&fakeSettings{}, nil)
	assert.Equal(t, pkgremoteflags.FlagName("inventory_drop_full_configuration"), h.FlagName())
	assert.Equal(t, []pkgremoteflags.FlagHandler{h}, h.Handlers())
	assert.True(t, h.IsHealthy())
}

func TestFullConfigurationEnabledMatrix(t *testing.T) {
	type userCase struct {
		name   string
		value  bool
		source model.Source
	}
	users := []userCase{
		{"unset-default", true, model.SourceDefault},
		{"unset-schema", true, model.SourceSchema},
		{"unset-unknown", true, model.SourceUnknown},
		{"file-true", true, model.SourceFile},
		{"file-false", false, model.SourceFile},
		{"env-false", false, model.SourceEnvVar},
		{"fleet-false", false, model.SourceFleetPolicies},
	}
	flags := []struct {
		name  string
		apply func(h *fullConfigFlagHandler)
	}{
		{"unknown", func(*fullConfigFlagHandler) {}},
		{"drop", func(h *fullConfigFlagHandler) { require.NoError(t, h.OnChange(true)) }},
		{"keep", func(h *fullConfigFlagHandler) { require.NoError(t, h.OnChange(false)) }},
	}

	for _, u := range users {
		for _, f := range flags {
			t.Run(fmt.Sprintf("%s/%s", u.name, f.name), func(t *testing.T) {
				h := newFullConfigFlagHandler(&fakeSettings{value: u.value, source: u.source}, nil)
				f.apply(h)

				var want bool
				switch {
				case u.source != model.SourceDefault && u.source != model.SourceSchema && u.source != model.SourceUnknown:
					want = u.value // user wins
				case f.name == "drop":
					want = false
				case f.name == "keep":
					want = true
				default:
					want = u.value
				}
				assert.Equal(t, want, h.fullConfigurationEnabled())
			})
		}
	}
}

func TestFullConfigFlagRefreshOnChange(t *testing.T) {
	refreshes := 0
	h := newFullConfigFlagHandler(&fakeSettings{value: true, source: model.SourceDefault}, func() { refreshes++ })

	require.NoError(t, h.OnChange(true)) // true -> false
	assert.Equal(t, 1, refreshes)
	require.NoError(t, h.OnChange(true)) // unchanged
	assert.Equal(t, 1, refreshes)
	require.NoError(t, h.OnChange(false)) // false -> true
	assert.Equal(t, 2, refreshes)
	require.NoError(t, h.OnChange(false)) // unchanged
	assert.Equal(t, 2, refreshes)
}

func TestFullConfigFlagNoRefreshWhenUserSet(t *testing.T) {
	refreshes := 0
	h := newFullConfigFlagHandler(&fakeSettings{value: true, source: model.SourceFile}, func() { refreshes++ })
	require.NoError(t, h.OnChange(true))
	h.OnNoConfig()
	assert.Zero(t, refreshes)
	assert.True(t, h.fullConfigurationEnabled())
}

func TestFullConfigFlagResets(t *testing.T) {
	resets := map[string]func(h *fullConfigFlagHandler){
		"OnNoConfig":  func(h *fullConfigFlagHandler) { h.OnNoConfig() },
		"SafeRecover": func(h *fullConfigFlagHandler) { h.SafeRecover(assert.AnError, true) },
	}
	for name, reset := range resets {
		t.Run(name, func(t *testing.T) {
			refreshes := 0
			h := newFullConfigFlagHandler(&fakeSettings{value: true, source: model.SourceDefault}, func() { refreshes++ })
			require.NoError(t, h.OnChange(true))
			require.False(t, h.fullConfigurationEnabled())
			assert.Equal(t, 1, refreshes)

			reset(h)
			assert.True(t, h.fullConfigurationEnabled())
			assert.Equal(t, 2, refreshes)

			reset(h) // already unknown: no refresh
			assert.Equal(t, 2, refreshes)
		})
	}
}

func getMetadata(t *testing.T, ia *inventoryagent) map[string]interface{} {
	t.Helper()
	return ia.getPayload().(*agentmetadata.Payload).Metadata
}

func TestComponentWiresFullConfigurationFlag(t *testing.T) {
	provides := getProvides(t, map[string]any{"inventories_configuration_enabled": true}, nil)
	ia := provides.Comp.(*inventoryagent)

	require.Len(t, provides.Subscriber.Handlers(), 1)
	h := provides.Subscriber.Handlers()[0]
	assert.Equal(t, pkgremoteflags.FlagName(flagDropFullConfiguration), h.FlagName())

	assert.Contains(t, getMetadata(t, ia), "full_configuration")
	require.NoError(t, h.OnChange(true))
	md := getMetadata(t, ia)
	assert.NotContains(t, md, "full_configuration")
	assert.Contains(t, md, "provided_configuration")
	require.NoError(t, h.OnChange(false))
	assert.Contains(t, getMetadata(t, ia), "full_configuration")
}
