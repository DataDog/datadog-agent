// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"context"
	"reflect"
	"sync"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log"
	tagger_api "github.com/DataDog/datadog-agent/comp/core/tagger/api"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/local"
	"github.com/DataDog/datadog-agent/comp/core/tagger/remote"
	"github.com/DataDog/datadog-agent/comp/core/tagger/replay"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Config configComponent.Component
	Log    logComp.Component
	Wmeta  workloadmeta.Component
	Params Params
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newTaggerClient,
		))
}

// tagger defines a tagger interface for internal usage of TaggerClient,
// so that Start and Stop methods are not exposed in component
// The structure of tagger component:
// Tagger Component
//
//	-> TaggerClient {captureTagger, defaultTagger}
//	                  -> tagger, is an interface with Start, Stop, Component methods (internal usage)
//	                        -> local, remote, replay, mock tagger etc.
type tagger interface {
	Start(ctx context.Context) error
	Stop() error
	Component
}

type datadogConfig struct {
	dogstatsdEntityIDPrecedenceEnabled bool
	dogstatsdOptOutEnabled             bool
}

// TaggerClient is a component that contains two tagger component: capturetagger and defaulttagger
//
// nolint:revive // TODO(containers) Fix revive linter
type TaggerClient struct {
	// captureTagger is a tagger instance that contains a tagger that will contain the tagger
	// state when replaying a capture scenario
	captureTagger tagger

	mux sync.RWMutex

	// defaultTagger is the shared tagger instance backing the global Tag and Init functions
	defaultTagger tagger

	wmeta         workloadmeta.Component
	datadogConfig datadogConfig
}

var (
	// ChecksCardinality defines the cardinality of tags we should send for check metrics
	// this can still be overridden when calling get_tags in python checks.
	ChecksCardinality collectors.TagCardinality

	// DogstatsdCardinality defines the cardinality of tags we should send for metrics from
	// dogstatsd.
	DogstatsdCardinality collectors.TagCardinality

	// we use to pull tagger metrics in dogstatsd. Pulling it later in the
	// pipeline improve memory allocation. We kept the old name to be
	// backward compatible and because origin detection only affect
	// dogstatsd metrics.
	tlmUDPOriginDetectionError = telemetry.NewCounter("dogstatsd", "udp_origin_detection_error",
		nil, "Dogstatsd UDP origin detection error count")
)

var _ Component = (*TaggerClient)(nil)

// newTaggerClient returns a Component based on provided params, once it is provided,
// fx will cache the component which is effectively a singleton instance, cached by fx.
// it should be deprecated and removed
func newTaggerClient(deps dependencies) Component {
	var taggerClient *TaggerClient
	switch deps.Params.agentTypeForTagger {
	case CLCRunnerRemoteTaggerAgent:
		options, err := remote.CLCRunnerOptions(deps.Config)
		if err != nil {
			deps.Log.Errorf("unable to deps.Configure the remote tagger: %s", err)
			taggerClient = &TaggerClient{
				defaultTagger: local.NewFakeTagger(),
				captureTagger: nil,
			}
		} else if options.Disabled {
			deps.Log.Errorf("remote tagger is disabled in clc runner.")
			taggerClient = &TaggerClient{
				defaultTagger: local.NewFakeTagger(),
				captureTagger: nil,
			}
		} else {
			taggerClient = &TaggerClient{
				defaultTagger: remote.NewTagger(options),
				captureTagger: nil,
			}
		}
	case NodeRemoteTaggerAgent:
		options, _ := remote.NodeAgentOptions(deps.Config)
		taggerClient = &TaggerClient{
			defaultTagger: remote.NewTagger(options),
			captureTagger: nil,
		}
	case LocalTaggerAgent:
		taggerClient = &TaggerClient{
			defaultTagger: local.NewTagger(deps.Wmeta),
			captureTagger: nil,
		}
	case FakeTagger:
		// all binaries are expected to provide their own tagger at startup. we
		// provide a fake tagger for testing purposes, as calling the global
		// tagger without proper initialization is very common there.
		taggerClient = &TaggerClient{
			defaultTagger: local.NewFakeTagger(),
			captureTagger: nil,
		}
	}

	if taggerClient != nil {
		taggerClient.wmeta = deps.Wmeta
	}

	taggerClient.datadogConfig.dogstatsdEntityIDPrecedenceEnabled = deps.Config.GetBool("dogstatsd_entity_id_precedence")
	taggerClient.datadogConfig.dogstatsdOptOutEnabled = deps.Config.GetBool("dogstatsd_origin_optout_enabled")

	deps.Log.Info("TaggerClient is created, defaultTagger type: ", reflect.TypeOf(taggerClient.defaultTagger))
	SetGlobalTaggerClient(taggerClient)
	deps.Lc.Append(fx.Hook{OnStart: func(c context.Context) error {
		var err error
		checkCard := deps.Config.GetString("checks_tag_cardinality")
		dsdCard := deps.Config.GetString("dogstatsd_tag_cardinality")
		ChecksCardinality, err = collectors.StringToTagCardinality(checkCard)
		if err != nil {
			deps.Log.Warnf("failed to parse check tag cardinality, defaulting to low. Error: %s", err)
			ChecksCardinality = collectors.LowCardinality
		}

		DogstatsdCardinality, err = collectors.StringToTagCardinality(dsdCard)
		if err != nil {
			deps.Log.Warnf("failed to parse dogstatsd tag cardinality, defaulting to low. Error: %s", err)
			DogstatsdCardinality = collectors.LowCardinality
		}
		// Main context passed to components, consistent with the one used in the workloadmeta component
		mainCtx, _ := common.GetMainCtxCancel()
		err = taggerClient.start(mainCtx)
		if err != nil && deps.Params.fallBackToLocalIfRemoteTaggerFails {
			deps.Log.Warnf("Starting remote tagger failed. Falling back to local tagger: %s", err)
			taggerClient.defaultTagger = local.NewTagger(deps.Wmeta)
			// Retry to start the local tagger
			return taggerClient.start(mainCtx)
		}
		return err
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error {
		return taggerClient.stop()
	}})
	return taggerClient
}

// start calls defaultTagger.Start
func (t *TaggerClient) start(ctx context.Context) error {
	return t.defaultTagger.Start(ctx)
}

// stop calls defaultTagger.Stop
func (t *TaggerClient) stop() error {
	return t.defaultTagger.Stop()
}

// GetDefaultTagger returns the default Tagger in current instance
func (t *TaggerClient) GetDefaultTagger() Component {
	return t.defaultTagger
}

// GetEntity returns the hash for the provided entity id.
func (t *TaggerClient) GetEntity(entityID string) (*types.Entity, error) {
	t.mux.RLock()
	if t.captureTagger != nil {
		entity, err := t.captureTagger.GetEntity(entityID)
		if err == nil && entity != nil {
			t.mux.RUnlock()
			return entity, nil
		}
	}
	t.mux.RUnlock()

	return t.defaultTagger.GetEntity(entityID)
}

// Tag queries the captureTagger (for replay scenarios) or the defaultTagger.
// It can return tags at high cardinality (with tags about individual containers),
// or at orchestrator cardinality (pod/task level).
func (t *TaggerClient) Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	// TODO: defer unlock once performance overhead of defer is negligible
	t.mux.RLock()
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Tag(entity, cardinality)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Tag(entity, cardinality)
}

// AccumulateTagsFor queries the defaultTagger to get entity tags from cache or
// sources and appends them to the TagsAccumulator.  It can return tags at high
// cardinality (with tags about individual containers), or at orchestrator
// cardinality (pod/task level).
func (t *TaggerClient) AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	// TODO: defer unlock once performance overhead of defer is negligible
	t.mux.RLock()
	if t.captureTagger != nil {
		err := t.captureTagger.AccumulateTagsFor(entity, cardinality, tb)
		if err == nil {
			t.mux.RUnlock()
			return nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.AccumulateTagsFor(entity, cardinality, tb)
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func (t *TaggerClient) GetEntityHash(entity string, cardinality collectors.TagCardinality) string {
	tags, err := t.Tag(entity, cardinality)
	if err != nil {
		return ""
	}
	return utils.ComputeTagsHash(tags)
}

// Standard queries the defaultTagger to get entity
// standard tags (env, version, service) from cache or sources.
func (t *TaggerClient) Standard(entity string) ([]string, error) {
	t.mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Standard(entity)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Standard(entity)
}

// AgentTags returns the agent tags
// It relies on the container provider utils to get the Agent container ID
func (t *TaggerClient) AgentTags(cardinality collectors.TagCardinality) ([]string, error) {
	ctrID, err := metrics.GetProvider(optional.NewOption(t.wmeta)).GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return nil, err
	}

	if ctrID == "" {
		return nil, nil
	}

	entityID := containers.BuildTaggerEntityName(ctrID)
	return t.Tag(entityID, cardinality)
}

// GlobalTags queries global tags that should apply to all data coming from the
// agent.
func (t *TaggerClient) GlobalTags(cardinality collectors.TagCardinality) ([]string, error) {
	t.mux.RLock()
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Tag(collectors.GlobalEntityID, cardinality)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Tag(collectors.GlobalEntityID, cardinality)
}

// globalTagBuilder queries global tags that should apply to all data coming
// from the agent and appends them to the TagsAccumulator
func (t *TaggerClient) globalTagBuilder(cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	t.mux.RLock()
	if t.captureTagger != nil {
		err := t.captureTagger.AccumulateTagsFor(collectors.GlobalEntityID, cardinality, tb)

		if err == nil {
			t.mux.RUnlock()
			return nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.AccumulateTagsFor(collectors.GlobalEntityID, cardinality, tb)
}

// List the content of the defaulTagger
func (t *TaggerClient) List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	return t.defaultTagger.List(cardinality)
}

// SetNewCaptureTagger sets the tagger to be used when replaying a capture
func (t *TaggerClient) SetNewCaptureTagger(newCaptureTagger *replay.Tagger) {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.captureTagger = newCaptureTagger
}

// ResetCaptureTagger resets the capture tagger to nil
func (t *TaggerClient) ResetCaptureTagger() {
	t.mux.Lock()
	defer t.mux.Unlock()
	t.captureTagger = nil
}

// EnrichTags extends a tag list with origin detection tags
// NOTE(remy): it is not needed to sort/dedup the tags anymore since after the
// enrichment, the metric and its tags is sent to the context key generator, which
// is taking care of deduping the tags while generating the context key.
func (t *TaggerClient) EnrichTags(tb tagset.TagsAccumulator, originInfo taggertypes.OriginInfo) {
	cardinality := taggerCardinality(originInfo.Cardinality)

	switch originInfo.ProductOrigin {
	case taggertypes.ProductOriginDogStatsD:
		// The following was moved from the dogstatsd package
		// originFromUDS is the origin discovered via UDS origin detection (container ID).
		// originFromTag is the origin sent by the client via the dd.internal.entity_id tag (non-prefixed pod uid).
		// originFromMsg is the origin sent by the client via the container field (non-prefixed container ID).
		// entityIDPrecedenceEnabled refers to the dogstatsd_entity_id_precedence parameter.
		//
		//	---------------------------------------------------------------------------------
		//
		// | originFromUDS | originFromTag | entityIDPrecedenceEnabled || Result: udsOrigin  |
		// |---------------|---------------|---------------------------||--------------------|
		// | any           | any           | false                     || originFromUDS      |
		// | any           | any           | true                      || empty              |
		// | any           | empty         | any                       || originFromUDS      |
		//
		//	---------------------------------------------------------------------------------
		//
		//	---------------------------------------------------------------------------------
		//
		// | originFromTag          | originFromMsg   || Result: originFromClient            |
		// |------------------------|-----------------||-------------------------------------|
		// | not empty && not none  | any             || pod prefix + originFromTag          |
		// | empty                  | empty           || empty                               |
		// | none                   | empty           || empty                               |
		// | empty                  | not empty       || container prefix + originFromMsg    |
		// | none                   | not empty       || container prefix + originFromMsg    |
		if t.datadogConfig.dogstatsdOptOutEnabled && originInfo.Cardinality == "none" {
			originInfo.FromUDS = packets.NoOrigin
			originInfo.FromTag = ""
			originInfo.FromMsg = ""
			return
		}

		// We use the UDS socket origin if no origin ID was specify in the tags
		// or 'dogstatsd_entity_id_precedence' is set to False (default false).
		if originInfo.FromUDS != packets.NoOrigin &&
			(originInfo.FromTag == "" || !t.datadogConfig.dogstatsdEntityIDPrecedenceEnabled) {
			if err := t.AccumulateTagsFor(originInfo.FromUDS, cardinality, tb); err != nil {
				log.Errorf(err.Error())
			}
		}

		// originFromClient can either be originInfo.FromTag or originInfo.FromMsg
		originFromClient := ""
		if originInfo.FromTag != "" && originInfo.FromTag != "none" {
			// Check if the value is not "none" in order to avoid calling the tagger for entity that doesn't exist.
			// Currently only supported for pods
			originFromClient = kubelet.KubePodTaggerEntityPrefix + originInfo.FromTag
		} else if originInfo.FromTag == "" && len(originInfo.FromMsg) > 0 {
			// originInfo.FromMsg is the container ID sent by the newer clients.
			originFromClient = containers.BuildTaggerEntityName(originInfo.FromMsg)
		}

		if originFromClient != "" {
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				tlmUDPOriginDetectionError.Inc()
				log.Tracef("Cannot get tags for entity %s: %s", originFromClient, err)
			}
		}
	default:
		if originInfo.FromUDS != packets.NoOrigin {
			if err := t.AccumulateTagsFor(originInfo.FromUDS, cardinality, tb); err != nil {
				log.Errorf(err.Error())
			}
		}

		if err := t.AccumulateTagsFor(containers.BuildTaggerEntityName(originInfo.FromMsg), cardinality, tb); err != nil {
			log.Tracef("Cannot get tags for entity %s: %s", originInfo.FromMsg, err)
		}

		if err := t.AccumulateTagsFor(kubelet.KubePodTaggerEntityPrefix+originInfo.FromTag, cardinality, tb); err != nil {
			log.Tracef("Cannot get tags for entity %s: %s", originInfo.FromMsg, err)
		}
	}

	if err := t.globalTagBuilder(cardinality, tb); err != nil {
		log.Error(err.Error())
	}

}

// taggerCardinality converts tagger cardinality string to collectors.TagCardinality
// It defaults to DogstatsdCardinality if the string is empty or unknown
func taggerCardinality(cardinality string) collectors.TagCardinality {
	if cardinality == "" {
		return DogstatsdCardinality
	}

	taggerCardinality, err := collectors.StringToTagCardinality(cardinality)
	if err != nil {
		log.Tracef("Couldn't convert cardinality tag: %v", err)
		return DogstatsdCardinality
	}

	return taggerCardinality
}

// Subscribe calls defaultTagger.Subscribe
func (t *TaggerClient) Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent {
	return t.defaultTagger.Subscribe(cardinality)
}

// Unsubscribe calls defaultTagger.Unsubscribe
func (t *TaggerClient) Unsubscribe(ch chan []types.EntityEvent) {
	t.defaultTagger.Unsubscribe(ch)
}

type optionalTaggerDeps struct {
	fx.In

	Lc     fx.Lifecycle
	Config configComponent.Component
	Log    logComp.Component
	Wmeta  optional.Option[workloadmeta.Component]
}

// OptionalModule defines the fx options when tagger should be used as an optional
func OptionalModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			NewOptionalTagger,
		),
	)
}

// NewOptionalTagger returns a tagger component if workloadmeta is available
func NewOptionalTagger(deps optionalTaggerDeps) optional.Option[Component] {
	w, ok := deps.Wmeta.Get()
	if !ok {
		return optional.NewNoneOption[Component]()
	}
	return optional.NewOption[Component](newTaggerClient(dependencies{
		In:     deps.In,
		Lc:     deps.Lc,
		Config: deps.Config,
		Log:    deps.Log,
		Wmeta:  w,
		Params: Params{
			agentTypeForTagger: LocalTaggerAgent,
		},
	}))
}
