// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package client

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type updaterFetcher struct {
	response *pbgo.ClientGetConfigsResponse
}

func (f *updaterFetcher) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return f.response, nil
}

// Exercise real repository updates and listener dispatch, not manual task redelivery.
func TestUpdaterReplaysTasksAfterCatalogChange(t *testing.T) {
	for _, withdraw := range []bool{false, true} {
		t.Run(fmt.Sprintf("withdraw=%t", withdraw), func(t *testing.T) {
			fetcher := &updaterFetcher{}
			c, err := NewClient(fetcher, WithUpdater(), WithoutTufVerification())
			require.NoError(t, err)
			defer c.Close()
			var order []string
			var executed []string
			catalogReady := false
			c.Subscribe(state.ProductInstallerConfig, func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
				order = append(order, "config")
			})
			c.Subscribe(state.ProductUpdaterCatalogDD, func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
				catalogReady = true
				order = append(order, "catalog")
			})
			c.Subscribe(state.ProductUpdaterTask, func(configs map[string]state.RawConfig, _ func(string, state.ApplyStatus)) {
				order = append(order, "task")
				if catalogReady {
					for _, config := range configs {
						executed = append(executed, string(config.Config))
					}
				}
			})

			const taskPath = "datadog/42/UPDATER_TASK/request/config"
			files := map[string]string{taskPath: `{"id":"request"}`}
			fetcher.response = updaterResponse(t, 1, files)
			require.NoError(t, c.update())
			require.Empty(t, executed)
			require.Equal(t, []string{"task"}, order)

			order = nil
			if withdraw {
				delete(files, taskPath)
			}
			files["datadog/42/UPDATER_CATALOG_DD/catalog/config"] = `{"packages":[]}`
			files["datadog/42/INSTALLER_CONFIG/deployment/config"] = `{"id":"deployment"}`
			fetcher.response = updaterResponse(t, 2, files)
			require.NoError(t, c.update())
			assert.Equal(t, []string{"config", "catalog", "task"}, order)
			if withdraw {
				assert.Empty(t, executed, "must not replay a cached, withdrawn task")
			} else {
				assert.Equal(t, []string{files[taskPath]}, executed)
			}

			order = nil
			require.NoError(t, c.update())
			assert.Empty(t, order, "unchanged products must not cause more notifications")
		})
	}
}

func TestNonUpdaterDoesNotReplayTasksAfterCatalogChange(t *testing.T) {
	fetcher := &updaterFetcher{}
	c, err := NewClient(fetcher, WithoutTufVerification())
	require.NoError(t, err)
	defer c.Close()
	calls := 0
	c.Subscribe(state.ProductUpdaterTask, func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
		calls++
	})
	files := map[string]string{"datadog/42/UPDATER_TASK/request/config": `{"id":"request"}`}
	fetcher.response = updaterResponse(t, 1, files)
	require.NoError(t, c.update())
	require.Equal(t, 1, calls)
	files["datadog/42/UPDATER_CATALOG_DD/catalog/config"] = `{"packages":[]}`
	fetcher.response = updaterResponse(t, 2, files)
	require.NoError(t, c.update())
	assert.Equal(t, 1, calls)
}

func updaterResponse(t *testing.T, version int, configs map[string]string) *pbgo.ClientGetConfigsResponse {
	t.Helper()
	targets := map[string]any{}
	response := &pbgo.ClientGetConfigsResponse{ConfigStatus: pbgo.ConfigStatus_CONFIG_STATUS_OK}
	for path, content := range configs {
		targets[path] = map[string]any{
			"length": len(content),
			"hashes": map[string]string{"sha256": fmt.Sprintf("%x", sha256.Sum256([]byte(content)))},
			"custom": map[string]int{"v": 1},
		}
		response.ClientConfigs = append(response.ClientConfigs, path)
		response.TargetFiles = append(response.TargetFiles, &pbgo.File{Path: path, Raw: []byte(content)})
	}
	var err error
	response.Targets, err = json.Marshal(map[string]any{
		"signed": map[string]any{
			"_type": "targets", "spec_version": "1.0", "version": version,
			"expires": "2100-01-01T00:00:00Z", "targets": targets,
		},
		"signatures": []any{},
	})
	require.NoError(t, err)
	return response
}
