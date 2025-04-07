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

	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestFetchConfig(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.Write([]byte(`{"key1": "value1"}`))
		}

		_, client, url := makeServer(t, handler)

		config, err := fetchConfig(context.Background(), client, "", url.String())
		require.NoError(t, err)
		require.Equal(t, map[string]interface{}{"key1": "value1"}, config)
	})

	t.Run("error", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}

		_, client, url := makeServer(t, handler)

		_, err := fetchConfig(context.Background(), client, "", url.String())
		require.Error(t, err)
	})

	t.Run("invalid reply", func(t *testing.T) {
		handler := func(w http.ResponseWriter, _ *http.Request) {
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

func TestUpdater(t *testing.T) {
	callbackCalled := 0
	handler := func(w http.ResponseWriter, _ *http.Request) {
		callbackCalled++
		w.Write([]byte(`{"key1": "value1"}`))
	}
	_, client, url := makeServer(t, handler)

	cfg := configmock.New(t)
	cfg.Set("key1", "base_value", model.SourceDefault)

	cs := configSync{
		Config:    cfg,
		Log:       logmock.New(t),
		Authtoken: authtokenmock.New(t),
		url:       url,
		client:    client,
		ctx:       context.Background(),
	}

	cs.updater()
	assert.Equal(t, "value1", cfg.Get("key1"))
	assert.Equal(t, 1, callbackCalled)

	cfg.Set("key1", "cli_value", model.SourceCLI)

	cs.updater()
	assert.Equal(t, "cli_value", cfg.Get("key1"))
	assert.Equal(t, 2, callbackCalled)
}
