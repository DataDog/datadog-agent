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

	"github.com/DataDog/datadog-agent/comp/api/authtoken"
	authtokenmock "github.com/DataDog/datadog-agent/comp/api/authtoken/mock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func makeDeps(t *testing.T) dependencies {
	return fxutil.Test[dependencies](t, fx.Options(
		config.MockModule(),
		fx.Supply(log.Params{}),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		telemetryimpl.MockModule(),
		fx.Supply(NewParams(0, false, 0)),
		authtokenmock.Module(),
	))
}

func makeConfigSync(deps dependencies) *configSync {
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
		client:    deps.Authtoken.GetClient(),
		ctx:       context.Background(),
	}
	return cs
}

func makeServer(t *testing.T, authtoken authtoken.Mock, handler http.HandlerFunc) (*httptest.Server, authtoken.SecureClient, *url.URL) {
	server := authtoken.NewMockServer(handler)

	url, err := url.Parse(server.URL)
	require.NoError(t, err)

	return server, authtoken.GetClient(), url
}

//nolint:revive
func makeConfigSyncWithServer(t *testing.T, ctx context.Context, handler http.HandlerFunc) *configSync {
	deps := makeDeps(t)
	cs := makeConfigSync(deps)

	authmock, ok := deps.Authtoken.(authtoken.Mock)
	require.True(t, ok)
	_, client, url := makeServer(t, authmock, handler)
	cs.ctx = ctx
	cs.client = client
	cs.url = url

	return cs
}

func assertConfigIsSet(t assert.TestingT, cfg model.Reader, key string, value interface{}) {
	assert.Equal(t, value, cfg.Get(key))
	assert.Equal(t, model.SourceLocalConfigProcess, cfg.GetSource(key))
}
