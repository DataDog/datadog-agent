// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ownerdetectionimpl provides the implementation of the owner detection client
package ownerdetectionimpl

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	ownerdetection "github.com/DataDog/datadog-agent/comp/ownerdetection/def"
	cache "github.com/DataDog/datadog-agent/comp/ownerdetection/store"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/common"
)

// Requires defines the dependencies for the owner detection client
type Requires struct {
	Lc        compdef.Lifecycle
	Config    config.Component
	Log       log.Component
	Wmeta     workloadmeta.Component
	Telemetry telemetry.Component
}

// Provides defines the output
type Provides struct {
	Comp ownerdetection.Component
}

// NewComponent returns a new owner detection client
func NewComponent(req Requires) (Provides, error) {

	ownerCache := cache.NewCache(0)

	cli, err := NewOwnerDetectionClient(req.Config, req.Wmeta, req.Log, req.Telemetry, ownerCache)
	if err != nil {
		return Provides{}, err
	}

	req.Log.Info("OwnerDetectionClient is created")
	req.Lc.Append(compdef.Hook{OnStart: func(_ context.Context) error {
		mainCtx, _ := common.GetMainCtxCancel()
		return cli.Start(mainCtx)
	}})
	req.Lc.Append(compdef.Hook{OnStop: func(context.Context) error {
		return cli.Stop()
	}})

	return Provides{Comp: cli}, nil
}

// NewOwnerDetectionClient returns a new owner detection client
func NewOwnerDetectionClient(cfg config.Component, wmeta workloadmeta.Component, log log.Component, telemetry telemetry.Component, ownerCache *cache.Cache) (ownerdetection.Component, error) {
	return &ownerDetectionClient{
		wmeta:         wmeta,
		log:           log,
		datadogConfig: cfg,
		telemetry:     telemetry,
		ownerCache:    ownerCache,
	}, nil
}

type ownerDetectionClient struct {
	wmeta         workloadmeta.Component
	datadogConfig config.Component
	log           log.Component
	telemetry     telemetry.Component
	ownerCache    *cache.Cache
}

// Start calls defaultTagger.Start
func (c *ownerDetectionClient) Start(ctx context.Context) error {
	c.log.Info("OwnerDetectionClient is started")
	go c.start(ctx)
	return nil
}

// Stop calls defaultTagger.Stop
func (c *ownerDetectionClient) Stop() error {
	return errors.New("Not implemented")
}

/*

// IDEA, store the DCAClient masked as ownwerDetectionClient in the struct itself

	if c.langDetectionCl == nil {
		// TODO: modify GetClusterAgentClient to accept a context with a deadline. If this
		// functions hangs forever, the component will be unhealthy and crash.
		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			return err
		}
		c.langDetectionCl = dcaClient
	}
*/
