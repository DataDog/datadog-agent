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
		fields:     map[string]interface{}{},
		collecting: make(chan struct{}, 1),
	}
	var metadataSerializer serializer.MetricSerializer
	if deps.Serializer != nil {
		metadataSerializer = submissionRecordingSerializer{MetricSerializer: deps.Serializer, onSubmitted: c.recordSubmission}
	}
	c.payload = util.CreateInventoryPayload(deps.Config, deps.Log, metadataSerializer, c.getPayload, "agent.json")
	c.payload.Enabled = c.payload.Enabled && serverlessdef.InventoryEligible(deps.Config, deps.Provider)
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

// collect is the runner callback: it waits for resource identity, records the
// inventory snapshot, and returns the interval the payload reports.
func (c *serverlessinitComp) collect(ctx context.Context) time.Duration {
	release, ok := c.enter(ctx)
	if !ok {
		return c.payload.MinInterval
	}
	defer release()
	data, ok := c.waitForIdentity(ctx)
	if !ok {
		return c.payload.MinInterval
	}
	c.applyInventoryData(data)
	return c.payload.Collect(ctx)
}

// enter serializes collection and shutdown behind a single slot, rejecting
// calls after stop. The stopped check runs after acquisition so stop cannot
// interleave between the check and the work. release is nil when ok is false.
func (c *serverlessinitComp) enter(ctx context.Context) (release func(), ok bool) {
	if ctx.Err() != nil || !c.payload.Enabled {
		return nil, false
	}
	select {
	case c.collecting <- struct{}{}:
	case <-ctx.Done():
		return nil, false
	}
	if c.stopped {
		<-c.collecting
		return nil, false
	}
	return func() { <-c.collecting }, true
}

// waitForIdentity polls the provider until it reports a resource identity,
// retrying at identityRetryInterval; ok is false when ctx ends first.
func (c *serverlessinitComp) waitForIdentity(ctx context.Context) (serverlessdef.ServerlessInitData, bool) {
	for ctx.Err() == nil {
		data := c.provider.GetInventoryData()
		if strings.TrimSpace(data.ResourceID) != "" {
			return data, true
		}
		select {
		case <-ctx.Done():
			return serverlessdef.ServerlessInitData{}, false
		case <-time.After(identityRetryInterval):
		}
	}
	return serverlessdef.ServerlessInitData{}, false
}

// applyInventoryData records a newly observed inventory snapshot: it triggers
// a payload refresh when the wire fields changed, rotates the host UUID when
// the resource identity changed, and constructs the base payload data on first
// use (NewData reads install info, so the constructor stays free of file reads).
func (c *serverlessinitComp) applyInventoryData(data serverlessdef.ServerlessInitData) {
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
}

func (c *serverlessinitComp) identityReady() bool {
	data := c.provider.GetInventoryData()
	return strings.TrimSpace(data.ResourceID) != ""
}

func (c *serverlessinitComp) recordSubmission() {
	c.submittedCurrentIdentity = true
}

// getPayload stamps the submission state as the report reason: the first
// payload for an identity is a startup report, later ones periodic.
func (c *serverlessinitComp) getPayload() marshaler.JSONMarshaler {
	fields := maps.Clone(c.fields)
	reason := reportReasonStartup
	if c.submittedCurrentIdentity {
		reason = reportReasonPeriodic
	}
	fields["report_reason"] = reason
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

// stop submits the final payload, spending up to shutdownGrace waiting for
// resource identity when the current identity has not been reported yet.
func (c *serverlessinitComp) stop(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()
	release, ok := c.enter(ctx)
	if !ok {
		return nil
	}
	defer release()
	c.stopped = true
	// The last known identity was already reported; do not spend the grace waiting.
	if c.submittedCurrentIdentity && !c.identityReady() {
		return nil
	}
	data, ok := c.waitForIdentity(ctx)
	if !ok {
		return nil
	}
	c.applyInventoryData(data)
	// The identity did not change since its last submission.
	if c.submittedCurrentIdentity {
		return nil
	}
	if err := c.payload.Submit(); err != nil {
		c.log.Errorf("unable to submit serverless-init inventory payload at shutdown: %s", err)
	}
	return nil
}
