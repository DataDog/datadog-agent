// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

type ownedTransport struct {
	http.RoundTripper
	closeIdle func()
}

func (t ownedTransport) CloseIdleConnections() { t.closeIdle() }

func (e *remoteConfigExtension) newAPIClient(ctx context.Context, host component.Host, cfg model.Reader, backendURL *url.URL) (*api.HTTPClient, error) {
	// Keep the pool's cleanup hook before adding OTel instrumentation, whose
	// transport does not expose CloseIdleConnections.
	settings := e.settings.TelemetrySettings
	settings.TracerProvider, settings.MeterProvider = nil, nil
	backend, err := e.config.Backend.ToClient(ctx, host.GetExtensions(), settings)
	if err != nil {
		return nil, err
	}
	closer, ok := backend.Transport.(interface{ CloseIdleConnections() })
	if !ok {
		return nil, errors.New("remote_config backend transport does not support connection cleanup")
	}
	backend.Transport = ownedTransport{
		RoundTripper: otelhttp.NewTransport(backend.Transport,
			otelhttp.WithTracerProvider(e.settings.TracerProvider),
			otelhttp.WithMeterProvider(e.settings.MeterProvider),
			otelhttp.WithPropagators(otel.GetTextMapPropagator())),
		closeIdle: closer.CloseIdleConnections,
	}
	return api.NewHTTPClient(api.Auth{APIKey: string(e.config.API.Key)}, cfg, backendURL,
		func(client *http.Client) { *client = *backend }, api.WithoutRedirects())
}

type boundedAPI struct {
	api.API
	lifetime context.Context
	timeout  time.Duration
}

func (b boundedAPI) requestContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithTimeout(parent, b.timeout)
	stop := context.AfterFunc(b.lifetime, cancel)
	return ctx, func() { stop(); cancel() }
}

func (b boundedAPI) Fetch(ctx context.Context, r *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	ctx, cancel := b.requestContext(ctx)
	defer cancel()
	return b.API.Fetch(ctx, r)
}

func (b boundedAPI) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	ctx, cancel := b.requestContext(ctx)
	defer cancel()
	return b.API.FetchOrgData(ctx)
}

func (b boundedAPI) FetchOrgStatus(ctx context.Context) (*pbgo.OrgStatusResponse, error) {
	ctx, cancel := b.requestContext(ctx)
	defer cancel()
	return b.API.FetchOrgStatus(ctx)
}
