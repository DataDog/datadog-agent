// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package serverlessinitimpl implements the serverless-init metadata provider.
package serverlessinitimpl

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/metadata/agentmetadata"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

const (
	identityRetryInterval    = 10 * time.Millisecond
	maxIdentityRetryInterval = time.Second
	shutdownGrace            = 250 * time.Millisecond
	reportNowTimeout         = 250 * time.Millisecond
)

type serverlessinitComp struct {
	payload                  util.InventoryPayload
	provider                 serverlessdef.CloudServiceMetadataProvider
	params                   serverlessdef.Params
	conf                     config.Component
	log                      log.Component
	hostname                 hostnameinterface.Component
	data                     *agentmetadata.Data
	fields                   map[string]interface{}
	uuid                     string
	lastResourceID           string
	submittedCurrentIdentity bool

	// slot is a one-element semaphore that collect, ReportNow, and stop hold
	// while they work. It guards the mutable fields above; waiters give up when
	// their context ends.
	slot chan struct{}
	// identityWake makes a waiting identity poll read the provider again at once.
	identityWake chan struct{}
	// submissions counts successful sends. ReportNow reads it before it waits
	// for the slot, to detect a payload that collection sent during the wait.
	submissions atomic.Uint64
	stopped     atomic.Bool
}

// Requires defines the dependencies for the serverlessinit component.
type Requires struct {
	Lc         compdef.Lifecycle
	Log        log.Component
	Config     config.Component
	Params     serverlessdef.Params
	Provider   serverlessdef.CloudServiceMetadataProvider
	Serializer serializer.MetricSerializer
	Hostname   hostnameinterface.Component
}

// Provides defines the output of the serverlessinit component.
type Provides struct {
	Comp     serverlessdef.Component
	Provider runnerdef.Provider
}

// NewComponent registers collection without reading metadata or submitting a payload.
func NewComponent(deps Requires) Provides {
	params := deps.Params
	params.Tags = maps.Clone(params.Tags)
	params.WrappedCommand = slices.Clone(params.WrappedCommand)
	c := &serverlessinitComp{
		provider:     deps.Provider,
		params:       params,
		conf:         deps.Config,
		log:          deps.Log,
		hostname:     deps.Hostname,
		fields:       map[string]interface{}{},
		slot:         make(chan struct{}, 1),
		identityWake: make(chan struct{}, 1),
	}
	metadataSerializer := submissionRecordingSerializer{MetricSerializer: deps.Serializer, onSubmitted: c.recordSubmission}
	c.payload = util.CreateInventoryPayload(deps.Config, deps.Log, metadataSerializer, c.getPayload, "agent.json")
	c.payload.Enabled = c.payload.Enabled && eligible(deps.Config, deps.Provider)
	provides := Provides{Comp: c}
	if c.payload.Enabled {
		deps.Config.OnUpdate(func(_ string, _ model.Source, _, _ any, _ uint64, _ model.Source) { c.payload.Refresh() })
		provides.Provider = runnerdef.NewProvider(c.collect)
		// Fx stops the runner before this hook and the serializer after it.
		deps.Lc.Append(compdef.Hook{OnStop: c.stop})
	}
	return provides
}

// collect is the runner callback: it waits for resource identity, records the
// metadata snapshot, and returns the interval the payload reports.
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
	c.applyMetadata(data)
	return c.payload.Collect(ctx)
}

// eligible reports whether the configuration opts in and the provider can
// collect metadata.
func eligible(conf model.Reader, provider serverlessdef.CloudServiceMetadataProvider) bool {
	return conf.GetBool("serverless.metadata_enabled") && provider.CanCollectMetadata()
}

// ReportNow applies the provider's current metadata and enqueues a startup
// payload at once. It does not repeat a payload for the current identity that
// collection sent while this call waited for the slot.
func (c *serverlessinitComp) ReportNow() {
	submissionsBeforeWait := c.submissions.Load()
	select {
	case c.identityWake <- struct{}{}:
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), reportNowTimeout)
	defer cancel()
	release, ok := c.enter(ctx)
	if !ok {
		if ctx.Err() != nil {
			c.log.Debugf("serverless-init metadata report skipped: collection did not finish within %s", reportNowTimeout)
		}
		return
	}
	defer release()
	data := c.provider.GetMetadata()
	if strings.TrimSpace(data.ResourceID) == "" {
		return
	}
	c.applyMetadata(data)
	if c.submittedCurrentIdentity && c.submissions.Load() != submissionsBeforeWait {
		return
	}
	// Report as startup even when this identity was already sent.
	c.submittedCurrentIdentity = false
	if err := c.payload.Submit(); err != nil {
		c.log.Errorf("unable to submit serverless-init metadata payload: %s", err)
	}
}

// enter acquires the slot and rejects the call after stop. The stopped check
// runs after acquisition so stop cannot start between the check and the work.
// release is nil when ok is false.
func (c *serverlessinitComp) enter(ctx context.Context) (release func(), ok bool) {
	release, ok = c.acquire(ctx)
	if ok && c.stopped.Load() {
		release()
		return nil, false
	}
	return release, ok
}

// acquire waits for the slot until ctx ends. release is nil when ok is false.
func (c *serverlessinitComp) acquire(ctx context.Context) (release func(), ok bool) {
	if ctx.Err() != nil || !c.payload.Enabled {
		return nil, false
	}
	select {
	case c.slot <- struct{}{}:
		return func() { <-c.slot }, true
	case <-ctx.Done():
		return nil, false
	}
}

// waitForIdentity polls the provider until it reports a resource identity.
// The delay between polls starts at identityRetryInterval and doubles up to
// maxIdentityRetryInterval; identityWake ends a delay early. ok is false when
// ctx ends first.
func (c *serverlessinitComp) waitForIdentity(ctx context.Context) (serverlessdef.CloudServiceMetadata, bool) {
	retry := identityRetryInterval
	for ctx.Err() == nil {
		data := c.provider.GetMetadata()
		if strings.TrimSpace(data.ResourceID) != "" {
			return data, true
		}
		select {
		case <-ctx.Done():
			return serverlessdef.CloudServiceMetadata{}, false
		case <-c.identityWake:
		case <-time.After(retry):
			retry = min(2*retry, maxIdentityRetryInterval)
		}
	}
	return serverlessdef.CloudServiceMetadata{}, false
}

// applyMetadata records a metadata snapshot. It refreshes the payload when the
// wire fields change and rotates the payload UUID when the resource ID changes.
// The base payload data is built on first use because Init reads install info.
func (c *serverlessinitComp) applyMetadata(data serverlessdef.CloudServiceMetadata) {
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
		c.data = agentmetadata.NewData(c.conf, c.log, c.hostname)
		c.data.Init()
	}
}

func (c *serverlessinitComp) identityReady() bool {
	data := c.provider.GetMetadata()
	return strings.TrimSpace(data.ResourceID) != ""
}

func (c *serverlessinitComp) recordSubmission() {
	c.submittedCurrentIdentity = true
	c.submissions.Add(1)
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

// stop rejects later reports. When the current identity was never sent, it
// waits up to shutdownGrace for an identity and submits a final payload.
func (c *serverlessinitComp) stop(ctx context.Context) error {
	c.stopped.Store(true)
	ctx, cancel := context.WithTimeout(ctx, shutdownGrace)
	defer cancel()
	release, ok := c.acquire(ctx)
	if !ok {
		return nil
	}
	defer release()
	// The last known identity was already reported; do not spend the grace waiting.
	if c.submittedCurrentIdentity && !c.identityReady() {
		return nil
	}
	data, ok := c.waitForIdentity(ctx)
	if !ok {
		return nil
	}
	c.applyMetadata(data)
	// The identity did not change since its last submission.
	if c.submittedCurrentIdentity {
		return nil
	}
	if err := c.payload.Submit(); err != nil {
		c.log.Errorf("unable to submit serverless-init metadata payload at shutdown: %s", err)
	}
	return nil
}
