// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

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
