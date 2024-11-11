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
	"strconv"
	"strings"
	"sync"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	taggerComp "github.com/DataDog/datadog-agent/comp/core/tagger"
	taggercommon "github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/local"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/remote"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/replay"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	coretelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/common"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"go.uber.org/fx"
)

const (
	// External Data Prefixes
	// These prefixes are used to build the External Data Environment Variable.
	// This variable is then used for Origin Detection.
	externalDataInitPrefix          = "it-"
	externalDataContainerNamePrefix = "cn-"
	externalDataPodUIDPrefix        = "pu-"
)

type externalData struct {
	init          bool
	containerName string
	podUID        string
}

type dependencies struct {
	fx.In

	Lc        fx.Lifecycle
	Config    config.Component
	Log       log.Component
	Wmeta     workloadmeta.Component
	Params    taggerComp.Params
	Telemetry coretelemetry.Component
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
	cfg           config.Component
	datadogConfig datadogConfig

	checksCardinality          types.TagCardinality
	dogstatsdCardinality       types.TagCardinality
	tlmUDPOriginDetectionError coretelemetry.Counter
	telemetryStore             *telemetry.Store

	log log.Component
}

func createTaggerClient(defaultTagger taggerComp.Component, l log.Component) *TaggerClient {
	return &TaggerClient{
		defaultTagger: defaultTagger,
		log:           l,
	}
}

// newTaggerClient returns a Component based on provided params, once it is provided,
// fx will cache the component which is effectively a singleton instance, cached by fx.
// it should be deprecated and removed
func newTaggerClient(deps dependencies) provides {
	var taggerClient *TaggerClient
	telemetryStore := telemetry.NewStore(deps.Telemetry)

	switch deps.Params.AgentTypeForTagger {
	case taggerComp.CLCRunnerRemoteTaggerAgent:
		options, err := remote.CLCRunnerOptions(deps.Config)

		if err != nil {
			deps.Log.Errorf("unable to deps.Configure the remote tagger: %s", err)
			taggerClient = createTaggerClient(local.NewFakeTagger(deps.Config, telemetryStore), deps.Log)
		} else if options.Disabled {
			deps.Log.Errorf("remote tagger is disabled in clc runner.")
			taggerClient = createTaggerClient(local.NewFakeTagger(deps.Config, telemetryStore), deps.Log)
		} else {
			filter := types.NewFilterBuilder().Exclude(types.KubernetesPodUID).Build(types.HighCardinality)
			taggerClient = createTaggerClient(remote.NewTagger(options, deps.Config, telemetryStore, filter), deps.Log)
		}
	case taggerComp.NodeRemoteTaggerAgent:
		options, _ := remote.NodeAgentOptions(deps.Config)
		taggerClient = createTaggerClient(remote.NewTagger(options, deps.Config, telemetryStore, types.NewMatchAllFilter()), deps.Log)
	case taggerComp.LocalTaggerAgent:
		taggerClient = createTaggerClient(local.NewTagger(deps.Config, deps.Wmeta, telemetryStore), deps.Log)
	case taggerComp.FakeTagger:
		// all binaries are expected to provide their own tagger at startup. we
		// provide a fake tagger for testing purposes, as calling the global
		// tagger without proper initialization is very common there.
		taggerClient = createTaggerClient(local.NewFakeTagger(deps.Config, telemetryStore), deps.Log)
	}

	if taggerClient != nil {
		taggerClient.wmeta = deps.Wmeta
	}

	taggerClient.datadogConfig.dogstatsdEntityIDPrecedenceEnabled = deps.Config.GetBool("dogstatsd_entity_id_precedence")
	taggerClient.datadogConfig.originDetectionUnifiedEnabled = deps.Config.GetBool("origin_detection_unified")
	taggerClient.datadogConfig.dogstatsdOptOutEnabled = deps.Config.GetBool("dogstatsd_origin_optout_enabled")
	// we use to pull tagger metrics in dogstatsd. Pulling it later in the
	// pipeline improve memory allocation. We kept the old name to be
	// backward compatible and because origin detection only affect
	// dogstatsd metrics.
	taggerClient.tlmUDPOriginDetectionError = deps.Telemetry.NewCounter("dogstatsd", "udp_origin_detection_error", nil, "Dogstatsd UDP origin detection error count")
	taggerClient.telemetryStore = telemetryStore

	deps.Log.Info("TaggerClient is created, defaultTagger type: ", reflect.TypeOf(taggerClient.defaultTagger))
	deps.Lc.Append(fx.Hook{OnStart: func(_ context.Context) error {
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
			taggerClient.defaultTagger = local.NewTagger(deps.Config, deps.Wmeta, telemetryStore)
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
		httputils.SetJSONError(w, t.log.Errorf("Unable to marshal tagger list response: %s", err), 500)
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

// ReplayTagger returns the replay tagger instance
func (t *TaggerClient) ReplayTagger() taggerComp.ReplayTagger {
	return replay.NewTagger(t.cfg, t.telemetryStore)
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *TaggerClient) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

// GetDefaultTagger returns the default Tagger in current instance
func (t *TaggerClient) GetDefaultTagger() taggerComp.Component {
	return t.defaultTagger
}

// GetEntity returns the hash for the provided entity id.
func (t *TaggerClient) GetEntity(entityID types.EntityID) (*types.Entity, error) {
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
func (t *TaggerClient) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	// TODO: defer unlock once performance overhead of defer is negligible
	t.mux.RLock()
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Tag(entityID, cardinality)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Tag(entityID, cardinality)
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *TaggerClient) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := taggercommon.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// AccumulateTagsFor queries the defaultTagger to get entity tags from cache or
// sources and appends them to the TagsAccumulator.  It can return tags at high
// cardinality (with tags about individual containers), or at orchestrator
// cardinality (pod/task level).
func (t *TaggerClient) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	// TODO: defer unlock once performance overhead of defer is negligible
	t.mux.RLock()
	if t.captureTagger != nil {
		err := t.captureTagger.AccumulateTagsFor(entityID, cardinality, tb)
		if err == nil {
			t.mux.RUnlock()
			return nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.AccumulateTagsFor(entityID, cardinality, tb)
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func (t *TaggerClient) GetEntityHash(entityID types.EntityID, cardinality types.TagCardinality) string {
	tags, err := t.Tag(entityID, cardinality)
	if err != nil {
		return ""
	}
	return utils.ComputeTagsHash(tags)
}

// Standard queries the defaultTagger to get entity
// standard tags (env, version, service) from cache or sources.
func (t *TaggerClient) Standard(entityID types.EntityID) ([]string, error) {
	t.mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Standard(entityID)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Standard(entityID)
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

	entityID := types.NewEntityID(types.ContainerID, ctrID)
	return t.Tag(entityID, cardinality)
}

// GlobalTags queries global tags that should apply to all data coming from the
// agent.
func (t *TaggerClient) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	t.mux.RLock()
	if t.captureTagger != nil {
		tags, err := t.captureTagger.Tag(taggercommon.GetGlobalEntityID(), cardinality)
		if err == nil && len(tags) > 0 {
			t.mux.RUnlock()
			return tags, nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.Tag(taggercommon.GetGlobalEntityID(), cardinality)
}

// globalTagBuilder queries global tags that should apply to all data coming
// from the agent and appends them to the TagsAccumulator
func (t *TaggerClient) globalTagBuilder(cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	t.mux.RLock()
	if t.captureTagger != nil {
		err := t.captureTagger.AccumulateTagsFor(taggercommon.GetGlobalEntityID(), cardinality, tb)

		if err == nil {
			t.mux.RUnlock()
			return nil
		}
	}
	t.mux.RUnlock()
	return t.defaultTagger.AccumulateTagsFor(taggercommon.GetGlobalEntityID(), cardinality, tb)
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
	cardinality := taggerCardinality(originInfo.Cardinality, t.dogstatsdCardinality, t.log)

	productOrigin := originInfo.ProductOrigin
	// If origin_detection_unified is disabled, we use DogStatsD's Legacy Origin Detection.
	// TODO: remove this when origin_detection_unified is enabled by default
	if !t.datadogConfig.originDetectionUnifiedEnabled && productOrigin == taggertypes.ProductOriginDogStatsD {
		productOrigin = taggertypes.ProductOriginDogStatsDLegacy
	}

	containerIDFromSocketCutIndex := len(types.ContainerID) + types.GetSeparatorLengh()

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
			originInfo.ContainerIDFromSocket = packets.NoOrigin
			originInfo.PodUID = ""
			originInfo.ContainerID = ""
			return
		}

		// We use the UDS socket origin if no origin ID was specify in the tags
		// or 'dogstatsd_entity_id_precedence' is set to False (default false).
		if originInfo.ContainerIDFromSocket != packets.NoOrigin &&
			(originInfo.PodUID == "" || !t.datadogConfig.dogstatsdEntityIDPrecedenceEnabled) &&
			len(originInfo.ContainerIDFromSocket) > containerIDFromSocketCutIndex {
			containerID := originInfo.ContainerIDFromSocket[containerIDFromSocketCutIndex:]
			originFromClient := types.NewEntityID(types.ContainerID, containerID)
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.log.Errorf("%s", err.Error())
			}
		}

		// originFromClient can either be originInfo.FromTag or originInfo.FromMsg
		var originFromClient types.EntityID
		if originInfo.PodUID != "" && originInfo.PodUID != "none" {
			// Check if the value is not "none" in order to avoid calling the tagger for entity that doesn't exist.
			// Currently only supported for pods
			originFromClient = types.NewEntityID(types.KubernetesPodUID, originInfo.PodUID)
		} else if originInfo.PodUID == "" && len(originInfo.ContainerID) > 0 {
			// originInfo.FromMsg is the container ID sent by the newer clients.
			originFromClient = types.NewEntityID(types.ContainerID, originInfo.ContainerID)
		}

		if !originFromClient.Empty() {
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.tlmUDPOriginDetectionError.Inc()
				t.log.Tracef("Cannot get tags for entity %s: %s", originFromClient, err)
			}
		}
	default:
		// Tag using Local Data
		if originInfo.ContainerIDFromSocket != packets.NoOrigin && len(originInfo.ContainerIDFromSocket) > containerIDFromSocketCutIndex {
			containerID := originInfo.ContainerIDFromSocket[containerIDFromSocketCutIndex:]
			originFromClient := types.NewEntityID(types.ContainerID, containerID)
			if err := t.AccumulateTagsFor(originFromClient, cardinality, tb); err != nil {
				t.log.Errorf("%s", err.Error())
			}
		}

		if err := t.AccumulateTagsFor(types.NewEntityID(types.ContainerID, originInfo.ContainerID), cardinality, tb); err != nil {
			t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.ContainerID, err)
		}

		if err := t.AccumulateTagsFor(types.NewEntityID(types.KubernetesPodUID, originInfo.PodUID), cardinality, tb); err != nil {
			t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.PodUID, err)
		}

		// Tag using External Data.
		// External Data is a list that contain prefixed-items, split by a ','. Current items are:
		// * "it-<init>" if the container is an init container.
		// * "cn-<container-name>" for the container name.
		// * "pu-<pod-uid>" for the pod UID.
		// Order does not matter.
		// Possible values:
		// * "it-false,cn-nginx,pu-3413883c-ac60-44ab-96e0-9e52e4e173e2"
		// * "cn-init,pu-cb4aba1d-0129-44f1-9f1b-b4dc5d29a3b3,it-true"
		if originInfo.ExternalData != "" {
			// Parse the external data and get the tags for the entity
			var parsedExternalData externalData
			var initParsingError error
			for _, item := range strings.Split(originInfo.ExternalData, ",") {
				switch {
				case strings.HasPrefix(item, externalDataInitPrefix):
					parsedExternalData.init, initParsingError = strconv.ParseBool(item[len(externalDataInitPrefix):])
					if initParsingError != nil {
						t.log.Tracef("Cannot parse bool from %s: %s", item[len(externalDataInitPrefix):], initParsingError)
					}
				case strings.HasPrefix(item, externalDataContainerNamePrefix):
					parsedExternalData.containerName = item[len(externalDataContainerNamePrefix):]
				case strings.HasPrefix(item, externalDataPodUIDPrefix):
					parsedExternalData.podUID = item[len(externalDataPodUIDPrefix):]
				}
			}

			// Accumulate tags for pod UID
			if parsedExternalData.podUID != "" {
				if err := t.AccumulateTagsFor(types.NewEntityID(types.KubernetesPodUID, parsedExternalData.podUID), cardinality, tb); err != nil {
					t.log.Tracef("Cannot get tags for entity %s: %s", originInfo.ContainerID, err)
				}
			}

			// Generate container ID from External Data
			generatedContainerID, err := t.generateContainerIDFromExternalData(parsedExternalData, metrics.GetProvider(optional.NewOption(t.wmeta)).GetMetaCollector())
			if err != nil {
				t.log.Tracef("Failed to generate container ID from %s: %s", originInfo.ExternalData, err)
			}

			// Accumulate tags for generated container ID
			if generatedContainerID != "" {
				if err := t.AccumulateTagsFor(types.NewEntityID(types.ContainerID, generatedContainerID), cardinality, tb); err != nil {
					t.log.Tracef("Cannot get tags for entity %s: %s", generatedContainerID, err)
				}
			}
		}
	}

	if err := t.globalTagBuilder(cardinality, tb); err != nil {
		t.log.Error(err.Error())
	}
}

// generateContainerIDFromExternalData generates a container ID from the external data
func (t *TaggerClient) generateContainerIDFromExternalData(e externalData, metricsProvider provider.ContainerIDForPodUIDAndContNameRetriever) (string, error) {
	return metricsProvider.ContainerIDForPodUIDAndContName(e.podUID, e.containerName, e.init, time.Second)
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
// It should be defaulted to DogstatsdCardinality if the string is empty or unknown
func taggerCardinality(cardinality string,
	defaultCardinality types.TagCardinality,
	l log.Component) types.TagCardinality {
	if cardinality == "" {
		return defaultCardinality
	}

	taggerCardinality, err := types.StringToTagCardinality(cardinality)
	if err != nil {
		l.Tracef("Couldn't convert cardinality tag: %v", err)
		return defaultCardinality
	}

	return taggerCardinality
}

// Subscribe calls defaultTagger.Subscribe
func (t *TaggerClient) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.defaultTagger.Subscribe(subscriptionID, filter)
}
