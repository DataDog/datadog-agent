// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package configsyncimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/api/authtoken/fetchonlyimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func makeDeps(t *testing.T) dependencies {
	return fxutil.Test[dependencies](t, fx.Options(
		core.MockBundle(),
		fetchonlyimpl.Module(),
	))
}

func makeConfigSync(t *testing.T) *configSync {
	deps := makeDeps(t)
	defaultURL := &url.URL{
		Scheme: "https",
		Host:   "localhost:1234",
		Path:   "/config/v1",
	}
	cs := &configSync{
		Config:    deps.Config,
		Log:       deps.Log,
		Authtoken: deps.Authtoken,
		url:       defaultURL,
		client:    http.DefaultClient,
		ctx:       context.Background(),
	}
	return cs
}

func makeServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *http.Client, *url.URL) {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	url, err := url.Parse(server.URL)
	require.NoError(t, err)

	return server, server.Client(), url
}

//nolint:revive
func makeConfigSyncWithServer(t *testing.T, ctx context.Context, handler http.HandlerFunc) *configSync {
	_, client, url := makeServer(t, handler)

	cs := makeConfigSync(t)
	cs.ctx = ctx
	cs.client = client
	cs.url = url

	return cs
}

func assertConfigIsSet(t assert.TestingT, cfg model.Reader, key string, value interface{}) {
	assert.Equal(t, value, cfg.Get(key))
	assert.Equal(t, model.SourceLocalConfigProcess, cfg.GetSource(key))
}
