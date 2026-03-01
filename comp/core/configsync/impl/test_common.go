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

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// testLifecycle is a simple lifecycle implementation for testing
type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(h compdef.Hook) {
	l.hooks = append(l.hooks, h)
}

func (l *testLifecycle) Start(ctx context.Context) error {
	for _, h := range l.hooks {
		if h.OnStart != nil {
			if err := h.OnStart(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (l *testLifecycle) Stop(ctx context.Context) error {
	for i := len(l.hooks) - 1; i >= 0; i-- {
		if l.hooks[i].OnStop != nil {
			if err := l.hooks[i].OnStop(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func makeDeps(t *testing.T) Requires {
	ipcComp := ipcmock.New(t)
	return Requires{
		Lc:        &testLifecycle{},
		Config:    config.NewMock(t),
		Log:       logmock.New(t),
		IPCClient: ipcComp.GetClient(),
		Params:    NewParams(0, false, 0),
	}
}

func makeConfigSync(deps Requires) *configSync {
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
