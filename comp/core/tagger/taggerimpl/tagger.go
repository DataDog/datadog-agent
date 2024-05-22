// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package taggerimpl contains the implementation of the tagger component.
package taggerimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/api/api"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log"
	taggerComp "github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/local"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/remote"
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
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/kubelet"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

var entityIDRegex = regexp.MustCompile(`^en-(init\.)?([a-fA-F0-9-]+)/([a-zA-Z0-9-_]+)$`)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Config configComponent.Component
	Log    logComp.Component
	Wmeta  workloadmeta.Component
	Params taggerComp.Params
}

type provides struct {
	fx.Out

	Comp     taggerComp.Component
	Endpoint api.AgentEndpointProvider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(
			newTaggerClient,
		))
}

type datadogConfig struct {
	dogstatsdEntityIDPrecedenceEnabled bool
	dogstatsdOptOutEnabled             bool
	originDetectionUnifiedEnabled      bool
}

// TaggerClient is a component that contains two tagger component: capturetagger and defaulttagger
//
// nolint:revive // TODO(containers) Fix revive linter
type TaggerClient struct {
	// captureTagger is a tagger instance that contains a tagger that will contain the tagger
	// state when replaying a capture scenario
	captureTagger taggerComp.Component

	mux sync.RWMutex

	// defaultTagger is the shared tagger instance backing the global Tag and Init functions
	defaultTagger taggerComp.Component

	wmeta         workloadmeta.Component
	datadogConfig datadogConfig

	checksCardinality    types.TagCardinality
	dogstatsdCardinality types.TagCardinality
}

// we use to pull tagger metrics in dogstatsd. Pulling it later in the
// pipeline improve memory allocation. We kept the old name to be
// backward compatible and because origin detection only affect
// dogstatsd metrics.
var tlmUDPOriginDetectionError = telemetry.NewCounter("dogstatsd", "udp_origin_detection_error",
	nil, "Dogstatsd UDP origin detection error count")

// newTaggerClient returns a Component based on provided params, once it is provided,
// fx will cache the component which is effectively a singleton instance, cached by fx.
// it should be deprecated and removed
func newTaggerClient(deps dependencies) provides {
	var taggerClient *TaggerClient
	switch deps.Params.AgentTypeForTagger {
	case taggerComp.CLCRunnerRemoteTaggerAgent:
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
	case taggerComp.NodeRemoteTaggerAgent:
		options, _ := remote.NodeAgentOptions(deps.Config)
		taggerClient = &TaggerClient{
			defaultTagger: remote.NewTagger(options),
			captureTagger: nil,
		}
	case taggerComp.LocalTaggerAgent:
		taggerClient = &TaggerClient{
			defaultTagger: local.NewTagger(deps.Wmeta),
			captureTagger: nil,
		}
	case taggerComp.FakeTagger:
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
	taggerClient.datadogConfig.originDetectionUnifiedEnabled = deps.Config.GetBool("origin_detection_unified")
	taggerClient.datadogConfig.dogstatsdOptOutEnabled = deps.Config.GetBool("dogstatsd_origin_optout_enabled")

	deps.Log.Info("TaggerClient is created, defaultTagger type: ", reflect.TypeOf(taggerClient.defaultTagger))
	taggerComp.SetGlobalTaggerClient(taggerClient)
	deps.Lc.Append(fx.Hook{OnStart: func(c context.Context) error {
		var err error
		checkCard := deps.Config.GetString("checks_tag_cardinality")
		dsdCard := deps.Config.GetString("dogstatsd_tag_cardinality")
		taggerClient.checksCardinality, err = types.StringToTagCardinality(checkCard)
		if err != nil {
			deps.Log.Warnf("failed to parse check tag cardinality, defaulting to low. Error: %s", err)
			taggerClient.checksCardinality = types.LowCardinality
		}

		taggerClient.dogstatsdCardinality, err = types.StringToTagCardinality(dsdCard)
		if err != nil {
			deps.Log.Warnf("failed to parse dogstatsd tag cardinality, defaulting to low. Error: %s", err)
			taggerClient.dogstatsdCardinality = types.LowCardinality
		}
		// Main context passed to components, consistent with the one used in the workloadmeta component
		mainCtx, _ := common.GetMainCtxCancel()
		err = taggerClient.Start(mainCtx)
		if err != nil && deps.Params.FallBackToLocalIfRemoteTaggerFails {
			deps.Log.Warnf("Starting remote tagger failed. Falling back to local tagger: %s", err)
			taggerClient.defaultTagger = local.NewTagger(deps.Wmeta)
			// Retry to start the local tagger
			return taggerClient.Start(mainCtx)
		}
		return err
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error {
		return taggerClient.Stop()
	}})
	return provides{
		Comp:     taggerClient,
		Endpoint: api.NewAgentEndpointProvider(taggerClient.writeList, "/tagger-list", "GET"),
	}
}

func (t *TaggerClient) writeList(w http.ResponseWriter, _ *http.Request) {
	response := t.List()

	jsonTags, err := json.Marshal(response)
	if err != nil {
		apiutils.SetJSONError(w, log.Errorf("Unable to marshal tagger list response: %s", err), 500)
		return
	}
	w.Write(jsonTags)
}

// Start calls defaultTagger.Start
func (t *TaggerClient) Start(ctx context.Context) error {
	return t.defaultTagger.Start(ctx)
}

// Stop calls defaultTagger.Stop
func (t *TaggerClient) Stop() error {
	return t.defaultTagger.Stop()
}

// GetDefaultTagger returns the default Tagger in current instance
func (t *TaggerClient) GetDefaultTagger() taggerComp.Component {
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
func (t *TaggerClient) Tag(entity string, cardinality types.TagCardinality) ([]string, error) {
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
func (t *TaggerClient) AccumulateTagsFor(entity string, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
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
func (t *TaggerClient) GetEntityHash(entity string, cardinality types.TagCardinality) string {
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
func (t *TaggerClient) AgentTags(cardinality types.TagCardinality) ([]string, error) {
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
func (t *TaggerClient) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
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
func (t *TaggerClient) globalTagBuilder(cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
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
func (t *TaggerClient) List() types.TaggerListResponse {
	return t.defaultTagger.List()
}

// SetNewCaptureTagger sets the tagger to be used when replaying a capture
func (t *TaggerClient) SetNewCaptureTagger(newCaptureTagger taggerComp.Component) {
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
	cardinality := t.taggerCardinality(originInfo.Cardinality)

	productOrigin := originInfo.ProductOrigin
	// If origin_detection_unified is disabled, we use DogStatsD's Legacy Origin Detection.
	// TODO: remove this when origin_detection_unified is enabled by default
	if !t.datadogConfig.originDetectionUnifiedEnabled && productOrigin == taggertypes.ProductOriginDogStatsD {
		productOrigin = taggertypes.ProductOriginDogStatsDLegacy
	}

	switch productOrigin {
	case taggertypes.ProductOriginDogStatsDLegacy:
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
			originFromClient = t.parseEntityID(originInfo.FromTag, metrics.GetProvider(optional.NewOption(t.wmeta)).GetMetaCollector())
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

// parseEntityID parses the entity ID and returns the correct tagger entity
// It can be either just a pod uid or `en-(init.)$(POD_UID)/$(CONTAINER_NAME)`
func (t *TaggerClient) parseEntityID(entityID string, metricsProvider provider.ContainerIDForPodUIDAndContNameRetriever) string {
	// Parse the (init.)$(POD_UID)/$(CONTAINER_NAME) entity ID with a regex
	parts := entityIDRegex.FindStringSubmatch(entityID)
	var cname, podUID string
	initCont := false
	switch len(parts) {
	case 0:
		return kubelet.KubePodTaggerEntityPrefix + entityID
	case 3:
		podUID = parts[1]
		cname = parts[2]
	case 4:
		podUID = parts[2]
		cname = parts[3]
		initCont = parts[1] == "init."
	}
	cid, err := metricsProvider.ContainerIDForPodUIDAndContName(
		podUID,
		cname,
		initCont,
		time.Second,
	)
	if err != nil {
		log.Debugf("Error getting container ID for pod UID and container name: %s", err)
		return entityID
	}
	return containers.BuildTaggerEntityName(cid)
}

// ChecksCardinality defines the cardinality of tags we should send for check metrics
// this can still be overridden when calling get_tags in python checks.
func (t *TaggerClient) ChecksCardinality() types.TagCardinality {
	return t.checksCardinality
}

// DogstatsdCardinality defines the cardinality of tags we should send for metrics from
// dogstatsd.
func (t *TaggerClient) DogstatsdCardinality() types.TagCardinality {
	return t.dogstatsdCardinality
}

// taggerCardinality converts tagger cardinality string to types.TagCardinality
// It defaults to DogstatsdCardinality if the string is empty or unknown
func (t *TaggerClient) taggerCardinality(cardinality string) types.TagCardinality {
	if cardinality == "" {
		return t.dogstatsdCardinality
	}

	taggerCardinality, err := types.StringToTagCardinality(cardinality)
	if err != nil {
		log.Tracef("Couldn't convert cardinality tag: %v", err)
		return t.dogstatsdCardinality
	}

	return taggerCardinality
}

// Subscribe calls defaultTagger.Subscribe
func (t *TaggerClient) Subscribe(cardinality types.TagCardinality) chan []types.EntityEvent {
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
func NewOptionalTagger(deps optionalTaggerDeps) optional.Option[taggerComp.Component] {
	w, ok := deps.Wmeta.Get()
	if !ok {
		return optional.NewNoneOption[taggerComp.Component]()
	}
	return optional.NewOption[taggerComp.Component](newTaggerClient(dependencies{
		In:     deps.In,
		Lc:     deps.Lc,
		Config: deps.Config,
		Log:    deps.Log,
		Wmeta:  w,
		Params: taggerComp.Params{
			AgentTypeForTagger: taggerComp.LocalTaggerAgent,
		},
	}).Comp)
}
