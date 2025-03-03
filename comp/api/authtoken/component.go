// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package authtoken implements the creation and access to the auth_token used to communicate between Agent processes.
// This component offers two implementations: one to create and fetch the auth_token and another that doesn't create the
// auth_token file but can fetch it it's available.
package authtoken

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// team: agent-runtimes

type ClientOption func(SecureClient) SecureClient
type RequestOption func(req *http.Request, onEnding func(func())) *http.Request

type SecureClient interface {
	Do(req *http.Request, opts ...RequestOption) (resp []byte, err error)
	Get(url string, opts ...RequestOption) (resp []byte, err error)
	Head(url string, opts ...RequestOption) (resp []byte, err error)
	Post(url string, contentType string, body io.Reader, opts ...RequestOption) (resp []byte, err error)
	PostChunk(url string, contentType string, body io.Reader, onChunk func([]byte), opts ...RequestOption) (err error)
	PostForm(url string, data url.Values, opts ...RequestOption) (resp []byte, err error)
	NewIPCEndpoint(endpointPath string) (*IPCEndpoint, error)
}

// Component is the component type.
type Component interface {
	Get() string
	GetTLSClientConfig() *tls.Config
	GetTLSServerConfig() *tls.Config
	HTTPMiddleware(next http.Handler) http.Handler
	GetClient(...ClientOption) SecureClient
}

func WithCloseConnection(req *http.Request, _ func(func())) *http.Request {
	req.Close = true
	return req
}

func WithLeaveConnectionOpen(req *http.Request, _ func(func())) *http.Request {
	req.Close = false
	return req
}

func WithContext(ctx context.Context) RequestOption {
	return func(req *http.Request, _ func(func())) *http.Request {
		req.WithContext(ctx)
		return req
	}
}

func WithTimeout(to time.Duration) RequestOption {
	return func(req *http.Request, onEnding func(func())) *http.Request {
		ctx, cncl := context.WithTimeout(context.Background(), to) // TODO IPC: handle call of WithContext and WithTimeout in the same time
		req.WithContext(ctx)
		onEnding(cncl)
		return req
	}
}

func WithValues(values url.Values) RequestOption {
	return func(req *http.Request, _ func(func())) *http.Request {
		req.URL.RawQuery = values.Encode()
		return req
	}
}

// NoneModule return a None optional type for authtoken.Component.
//
// This helper allows code that needs a disabled Optional type for authtoken to get it. The helper is split from
// the implementation to avoid linking with the dependencies from sysprobeconfig.
func NoneModule() fxutil.Module {
	return fxutil.Component(fx.Provide(func() option.Option[Component] {
		return option.None[Component]()
	}))
}
