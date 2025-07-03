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

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"

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
		fx.Provide(func(t testing.TB) ipc.Component { return ipcmock.New(t) }),
		fx.Provide(func(ipcComp ipc.Component) ipc.HTTPClient { return ipcComp.GetClient() }),
		fx.Supply(NewParams(0, false, 0)),
	))
}

func makeConfigSync(deps dependencies) *configSync {
	defaultURL := &url.URL{
		Scheme: "https",
		Host:   "localhost:1234",
		Path:   "/config/v1",
	}
	cs := &configSync{
		Config: deps.Config,
		Log:    deps.Log,
		url:    defaultURL,
		client: deps.IPCClient,
		ctx:    context.Background(),
	}
	return cs
}

func makeServer(t *testing.T, ipcmock *ipcmock.IPCMock, handler http.HandlerFunc) (*httptest.Server, *url.URL) {
	server := ipcmock.NewMockServer(handler)

	url, err := url.Parse(server.URL)
	require.NoError(t, err)

	return server, url
}

//nolint:revive
func makeConfigSyncWithServer(t *testing.T, ctx context.Context, handler http.HandlerFunc) *configSync {
	deps := makeDeps(t)

	ipcmock := ipcmock.New(t)
	_, url := makeServer(t, ipcmock, handler)

	cs := makeConfigSync(deps)
	cs.ctx = ctx
	cs.url = url

	return cs
}

func assertConfigIsSet(t assert.TestingT, cfg model.Reader, key string, value interface{}) {
	assert.Equal(t, value, cfg.Get(key))
	assert.Equal(t, model.SourceLocalConfigProcess, cfg.GetSource(key))
}
