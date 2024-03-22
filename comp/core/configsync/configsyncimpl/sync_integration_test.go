// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package configsyncimpl

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestRunWithChan(t *testing.T) {
	t.Run("server error", func(t *testing.T) {
		var called bool
		handler := func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusInternalServerError)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cs := makeConfigSyncWithServer(t, ctx, handler)

		ch := make(chan time.Time, 1)
		ch <- time.Now()
		time.AfterFunc(100*time.Millisecond, cancel)
		cs.runWithChan(ch)

		require.True(t, called)
	})

	t.Run("success", func(t *testing.T) {
		var called bool
		handler := func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.Write([]byte(`{"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4"}`))
		}

		ctx, cancel := context.WithCancel(context.Background())
		cs := makeConfigSyncWithServer(t, ctx, handler)

		cs.Config.Set("key0", "value0", pkgconfigmodel.SourceFile)
		cs.Config.Set("key1", "value1", pkgconfigmodel.SourceFile)
		cs.Config.Set("key2", "not-value2", pkgconfigmodel.SourceFile)
		cs.Config.Set("key4", "set-with-cli", pkgconfigmodel.SourceCLI)

		ch := make(chan time.Time, 1)
		ch <- time.Now()
		time.AfterFunc(100*time.Millisecond, cancel)
		cs.runWithChan(ch)

		require.True(t, called)

		assert.Equal(t, "value0", cs.Config.GetString("key0"))
		assertConfigIsSet(t, cs.Config, "key1", "value1")
		assertConfigIsSet(t, cs.Config, "key2", "value2")
		assertConfigIsSet(t, cs.Config, "key3", "value3")
		assert.Equal(t, "set-with-cli", cs.Config.GetString("key4"))
	})

	t.Run("eventual success", func(t *testing.T) {
		const errnums = 3
		callnb := atomic.NewInt32(0)
		handler := func(w http.ResponseWriter, r *http.Request) {
			nb := callnb.Inc()
			if nb <= errnums {
				w.WriteHeader(http.StatusInternalServerError)
			} else {
				w.Write([]byte(`{"key1": "value1"}`))
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		cs := makeConfigSyncWithServer(t, ctx, handler)

		cs.Config.Set("key1", "not-value1", pkgconfigmodel.SourceFile)

		ch := make(chan time.Time, 1)
		go cs.runWithChan(ch)

		for i := 1; i <= errnums; i++ {
			ch <- time.Now()
			time.Sleep(50 * time.Millisecond)
			require.EqualValues(t, i, callnb.Load())
			require.Equal(t, "not-value1", cs.Config.GetString("key1"))
		}

		ch <- time.Now()
		time.Sleep(50 * time.Millisecond)
		require.EqualValues(t, errnums+1, callnb.Load())
		assertConfigIsSet(t, cs.Config, "key1", "value1")
	})
}

func TestRunWithInterval(t *testing.T) {
	configCore := pkgconfigmodel.NewConfig("test", "DD", strings.NewReplacer(".", "_"))
	configCore.Set("api_key", "api_key_core1", pkgconfigmodel.SourceFile)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := configCore.GetString("api_key")
		w.Write([]byte(fmt.Sprintf(`{"api_key": "%s"}`, key)))
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cs := makeConfigSyncWithServer(t, ctx, handler)
	cs.Config.Set("api_key", "api_key_remote", pkgconfigmodel.SourceEnvVar)

	refreshInterval := time.Millisecond * 200
	maxWaitInterval := 5 * refreshInterval

	t.Log("Starting config server")
	go cs.runWithInterval(refreshInterval)

	t.Log("Waiting for the first config sync")
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assertConfigIsSet(t, cs.Config, "api_key", "api_key_core1")
	}, maxWaitInterval, refreshInterval)

	t.Log("Updating api_key in the core config")
	configCore.Set("api_key", "api_key_core2", pkgconfigmodel.SourceAgentRuntime)
	t.Log("Waiting for the next config sync")
	require.EventuallyWithT(t, func(t *assert.CollectT) {
		assertConfigIsSet(t, cs.Config, "api_key", "api_key_core2")
	}, maxWaitInterval, refreshInterval)
}
