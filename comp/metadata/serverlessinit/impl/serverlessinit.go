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
	"github.com/DataDog/datadog-agent/comp/metadata/inventorypayload"
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
	payload                  util.InventoryPayload
	provider                 serverlessdef.InventoryProvider
	params                   serverlessdef.Params
	conf                     config.Component
	log                      log.Component
	hostname                 hostnameinterface.Component
	data                     *inventorypayload.Data
	fields                   map[string]interface{}
	uuid                     string
	lastResourceID           string
	submittedCurrentIdentity bool
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
	var metadataSerializer serializer.MetricSerializer
	if deps.Serializer != nil {
		metadataSerializer = submissionRecordingSerializer{MetricSerializer: deps.Serializer, onSubmitted: c.recordSubmission}
	}
	c.payload = util.CreateInventoryPayload(deps.Config, deps.Log, metadataSerializer, c.getPayload, "agent.json")
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
	}

	for ctx.Err() == nil {
		data := c.provider.GetInventoryData()
		if strings.TrimSpace(data.ResourceID) == "" {
			// The last known identity was already reported; do not spend the shutdown grace waiting.
			if final && c.submittedCurrentIdentity {
				return c.payload.MinInterval
			}
			select {
			case <-ctx.Done():
				return c.payload.MinInterval
			case <-time.After(identityRetryInterval):
			}
			continue
		}
		fields := buildFields(data, c.params, c.conf)
		if !reflect.DeepEqual(fields, c.fields) {
			c.payload.Refresh()
		}
		c.fields = fields
		if data.ResourceID != c.lastResourceID {
			c.uuid = uuid.New().String()
			c.lastResourceID = data.ResourceID
			c.submittedCurrentIdentity = false
		}
		if c.data == nil {
			c.data = inventorypayload.NewData(c.conf, c.log, c.hostname)
		}
		if ctx.Err() != nil {
			return c.payload.MinInterval
		}
		if !final {
			return c.payload.Collect(ctx)
		}
		if c.submittedCurrentIdentity {
			return c.payload.MinInterval
		}
		if err := c.payload.Submit(); err != nil {
			c.log.Errorf("unable to submit serverless-init inventory payload at shutdown: %s", err)
		}
		return c.payload.MinInterval
	}
	return c.payload.MinInterval
}

func (c *serverlessinitComp) recordSubmission() {
	c.submittedCurrentIdentity = true
}

func (c *serverlessinitComp) getPayload() marshaler.JSONMarshaler {
	fields := maps.Clone(c.fields)
	if c.submittedCurrentIdentity {
		fields["report_reason"] = reportReasonPeriodic
	}
	return c.data.GetPayload(fields, c.uuid)
}

type submissionRecordingSerializer struct {
	serializer.MetricSerializer
	onSubmitted func()
}

func (s submissionRecordingSerializer) SendMetadata(payload marshaler.JSONMarshaler) error {
	if err := s.MetricSerializer.SendMetadata(payload); err != nil {
		return err
	}
	s.onSubmitted()
	return nil
}

func (c *serverlessinitComp) stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()
	c.collectPayload(ctx, true)
	return nil
}
