// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configsyncimpl

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestFetchConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"key1": "value1"}`))
		}

		_, client, url := makeServer(t, handler)

		config, err := fetchConfig(context.Background(), client, "", url.String())
		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{"key1": "value1"}, config)
	})

	t.Run("error", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, client, url := makeServer(t, handler)

		_, err := fetchConfig(context.Background(), client, "", url.String())
		require.Error(t, err)
	})

	t.Run("invalid reply", func(t *testing.T) {
		handler := func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("invalid json"))
		}

		_, client, url := makeServer(t, handler)

		_, err := fetchConfig(context.Background(), client, "", url.String())
		require.Error(t, err)
	})

	t.Run("context cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, client, url := makeServer(t, nil)

		_, err := fetchConfig(ctx, client, "", url.String())
		require.Error(t, err)
	})
}

func TestUpdateConfig(t *testing.T) {
	cfg := pkgconfigmodel.NewConfig("test", "DD", nil)
	cfg.Set("key1", "value1", pkgconfigmodel.SourceFile)
	cfg.Set("key3", "set-with-cli", pkgconfigmodel.SourceCLI)

	assert.False(t, updateConfig(cfg, "key1", "value1"))
	assert.True(t, updateConfig(cfg, "key2", "value2"))
	assert.False(t, updateConfig(cfg, "key3", "value3"))
}
