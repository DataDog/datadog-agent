// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package serverlessinitimpl implements the serverless-init inventory metadata provider.
package serverlessinitimpl

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	identityRetryInterval = 10 * time.Millisecond
	shutdownGrace         = 250 * time.Millisecond
)

type serverlessinitComp struct {
	payload             util.InventoryPayload
	provider            serverlessdef.InventoryProvider
	params              serverlessdef.Params
	conf                config.Component
	log                 log.Component
	hostname            hostnameinterface.Component
	data                *inventoryagent.Data
	fields              map[string]interface{}
	uuid                string
	lastResourceID      string
	submittedResourceID string
	// Serializes collection and shutdown without an uninterruptible lock wait.
	collecting chan struct{}
	stopped    bool
}

// Requires defines the dependencies for the serverlessinit component.
type Requires struct {
	Lc         compdef.Lifecycle
	Log        log.Component
	Config     config.Component
	Params     serverlessdef.Params
	Provider   serverlessdef.InventoryProvider
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

// Provides defines the output of the serverlessinit component.
type Provides struct {
	Comp     serverlessdef.Component
	Provider runnerdef.Provider
}

// NewComponent registers collection without reading inventory data or submitting a payload.
func NewComponent(deps Requires) Provides {
	params := deps.Params
	params.Tags = maps.Clone(params.Tags)
	params.WrappedCommand = slices.Clone(params.WrappedCommand)
	c := &serverlessinitComp{
		provider:   deps.Provider,
		params:     params,
		conf:       deps.Config,
		log:        deps.Log,
		hostname:   deps.Hostname,
		collecting: make(chan struct{}, 1),
	}
	c.payload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, c.getPayload, "agent.json")
	c.payload.Enabled = c.payload.Enabled && serverlessdef.InventoryEligible(deps.Config, deps.Provider)
	c.payload.FirstRunDelay = 0
	provides := Provides{Comp: c}
	if c.payload.Enabled {
		deps.Config.OnUpdate(func(_ string, _ model.Source, _, _ any, _ uint64, _ model.Source) { c.payload.Refresh() })
		provides.Provider = runnerdef.NewProvider(c.collect)
		// The runner depends on this provider; it stops before this hook, and
		// the serializer's dependencies drain after it.
		deps.Lc.Append(compdef.Hook{OnStop: c.stop})
	}
	return provides
}

func (c *serverlessinitComp) collect(ctx context.Context) time.Duration {
	return c.collectPayload(ctx, false)
}

func (c *serverlessinitComp) collectPayload(ctx context.Context, final bool) time.Duration {
	if ctx.Err() != nil || !c.payload.Enabled {
		return c.payload.MinInterval
	}
	select {
	case c.collecting <- struct{}{}:
		defer func() { <-c.collecting }()
	case <-ctx.Done():
		return c.payload.MinInterval
	}
	if c.stopped {
		return c.payload.MinInterval
	}
	if final {
		c.stopped = true
		if !c.payload.LastSubmit.IsZero() {
			return c.payload.MinInterval
		}
	}

	for ctx.Err() == nil {
		data := c.provider.GetInventoryData()
		if strings.TrimSpace(data.ResourceID) != "" {
			fields := buildFields(data, c.params, c.conf)
			if !reflect.DeepEqual(fields, c.fields) {
				c.payload.Refresh()
			}
			c.fields = fields
			if data.ResourceID != c.lastResourceID {
				c.uuid = uuid.New().String()
				c.lastResourceID = data.ResourceID
			}
			if c.data == nil {
				c.data = inventoryagent.NewData(c.conf, c.log, c.hostname)
			}
			if ctx.Err() != nil {
				return c.payload.MinInterval
			}
			lastSubmit := c.payload.LastSubmit
			if final {
				c.payload.Submit()
			} else {
				c.payload.Collect(ctx)
			}
			if !c.payload.LastSubmit.Equal(lastSubmit) {
				c.submittedResourceID = data.ResourceID
			}
			return c.payload.MinInterval
		}
		select {
		case <-ctx.Done():
			return c.payload.MinInterval
		case <-time.After(identityRetryInterval):
		}
	}
	return c.payload.MinInterval
}

func (c *serverlessinitComp) getPayload() marshaler.JSONMarshaler {
	fields := maps.Clone(c.fields)
	if c.submittedResourceID == c.lastResourceID {
		fields["report_reason"] = reportReasonPeriodic
	}
	return c.data.GetPayload(fields, c.uuid)
}

func (c *serverlessinitComp) stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()
	c.collectPayload(ctx, true)
	return nil
}
