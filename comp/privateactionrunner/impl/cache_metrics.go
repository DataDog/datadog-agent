// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !kubeapiserver

package privateactionrunnerimpl

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/authoredscripts"
)

func (p *PrivateActionRunner) startCacheMetrics(ctx context.Context, client statsd.ClientInterface) {
	stop, err := authoredscripts.StartCacheMetrics(context.WithoutCancel(ctx), client)
	if err != nil {
		p.logger.Warnf("Could not start authored-script cache metrics: %v", err)
		return
	}
	p.stopCacheMetrics = stop
}
