// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package tracer

import (
	"fmt"
	"testing"

	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessCacheHandleProcessEvent(t *testing.T) {

	const ddService = "DD_SERVICE"
	const ddVersion = "DD_VERSION"
	const ddEnv = "DD_ENV"

	envs := map[string]string{
		ddService: "service",
		ddVersion: "version",
		ddEnv:     "env",
	}

	tests := []struct {
		envs        []string
		filter      []string
		filtered    []string
		containerID string
	}{
		{},
		{envs: nil, filter: defaultFilteredEnvs, filtered: nil},
		{envs: []string{ddEnv}, filter: defaultFilteredEnvs, filtered: []string{ddEnv}},
		{envs: []string{ddVersion}, filter: defaultFilteredEnvs, filtered: []string{ddVersion}},
		{envs: []string{ddService}, filter: defaultFilteredEnvs, filtered: []string{ddService}},
		{envs: []string{ddEnv, ddVersion}, filter: defaultFilteredEnvs, filtered: []string{ddEnv, ddVersion}},
		{envs: []string{ddEnv, ddService}, filter: defaultFilteredEnvs, filtered: []string{ddEnv, ddService}},
		{envs: []string{ddVersion, ddService}, filter: defaultFilteredEnvs, filtered: []string{ddVersion, ddService}},
		{envs: []string{ddService, ddVersion, ddEnv}, filter: defaultFilteredEnvs, filtered: defaultFilteredEnvs},
		{envs: []string{ddService, ddVersion, ddEnv, "foo=bar"}, filter: defaultFilteredEnvs, filtered: defaultFilteredEnvs},
		{envs: []string{"foo"}, filter: defaultFilteredEnvs, filtered: []string{}},
		{envs: []string{ddEnv}},
		{envs: []string{ddVersion}},
		{envs: []string{ddService}},
		{envs: []string{ddEnv, ddVersion}},
		{envs: []string{ddEnv, ddService}},
		{envs: []string{ddVersion, ddService}},
		{envs: []string{ddService, ddVersion, ddEnv}},
	}

	testFunc := func(t *testing.T, entry *smodel.ProcessCacheEntry) {
		for i, te := range tests {
			t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
				pc, err := newProcessCache(10, te.filter)
				require.NoError(t, err)

				entry.EnvsEntry.Values = nil
				for _, e := range te.envs {
					entry.EnvsEntry.Values = append(entry.EnvsEntry.Values, e+"="+envs[e])
				}
				pc.handleProcessEvent(entry)

				p, ok := pc.Process(entry.Pid)
				if entry.ContainerID == "" && len(te.filter) > 0 && len(te.filtered) == 0 {
					assert.False(t, ok)
					assert.Nil(t, p)
				} else {
					assert.True(t, ok)
					assert.NotNil(t, p)
					assert.Equal(t, entry.Pid, p.Pid)
					if entry.ContainerID != "" {
						assert.Equal(t, entry.ContainerID, p.ContainerId)
					}
					l := te.envs
					if len(te.filter) > 0 {
						l = te.filtered
					}
					assert.Len(t, p.Envs, len(l))
					for _, e := range l {
						assert.Contains(t, p.Envs, e)
						assert.Equal(t, envs[e], p.Envs[e])
					}
				}
			})
		}
	}

	t.Run("without container id", func(t *testing.T) {
		entry := smodel.ProcessCacheEntry{
			ProcessContext: smodel.ProcessContext{
				Process: smodel.Process{
					PIDContext: smodel.PIDContext{
						Pid: 1234,
					},
					EnvsEntry: &smodel.EnvsEntry{},
				},
			},
		}

		testFunc(t, &entry)
	})

	t.Run("with container id", func(t *testing.T) {
		entry := smodel.ProcessCacheEntry{
			ProcessContext: smodel.ProcessContext{
				Process: smodel.Process{
					PIDContext: smodel.PIDContext{
						Pid: 1234,
					},
					ContainerID: "container",
					EnvsEntry:   &smodel.EnvsEntry{},
				},
			},
		}

		testFunc(t, &entry)
	})
}
