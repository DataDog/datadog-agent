// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"context"
	"sync"

	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	logComp "github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/tagger/local"
	"github.com/DataDog/datadog-agent/comp/core/tagger/remote"
	"github.com/DataDog/datadog-agent/comp/core/tagger/replay"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	tagger_api "github.com/DataDog/datadog-agent/pkg/tagger/api"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In

	Lc      fx.Lifecycle
	Config  configComponent.Component
	Log     logComp.Component
	Context context.Context
	wmeta   workloadmeta.Component
	Params  Params
}

var (
	// defaultTagger is the shared tagger instance backing the global Tag and Init functions
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	defaultTagger Component
)

// captureTagger is a tagger instance that contains a tagger that will contain the tagger
// state when replaying a capture scenario
// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
var (
	captureTagger Component
	mux           sync.RWMutex
)

// ChecksCardinality defines the cardinality of tags we should send for check metrics
// this can still be overridden when calling get_tags in python checks.
var ChecksCardinality collectors.TagCardinality

// DogstatsdCardinality defines the cardinality of tags we should send for metrics from
// dogstatsd.
var DogstatsdCardinality collectors.TagCardinality

// we use to pull tagger metrics in dogstatsd. Pulling it later in the
// pipeline improve memory allocation. We kept the old name to be
// backward compatible and because origin detection only affect
// dogstatsd metrics.
var tlmUDPOriginDetectionError = telemetry.NewCounter("dogstatsd", "udp_origin_detection_error",
	nil, "Dogstatsd UDP origin detection error count")

// newTagger returns a Component based on provided params, once it is provided,
// fx will cache the component which is effectively a singleton instance, cached by fx.
// TODO(components) (tagger): defaultTagger is a legacy global variable but still in use, to be eliminated
// it should be deprecated and removed
func newTagger(deps dependencies) Component {
	switch deps.Params.TaggerType {
	case RemoteTagger:
		if deps.Params.AgentType == CLCRunnerRemoteTaggerAgent {
			options, err := remote.CLCRunnerOptions(deps.Config)
			if err != nil {
				deps.Log.Errorf("unable to deps.Configure the remote tagger: %s", err)
				defaultTagger = local.NewFakeTagger()
			} else if options.Disabled {
				deps.Log.Errorf("remote tagger is disabled in clc runner.")
				defaultTagger = local.NewFakeTagger()
			} else {
				defaultTagger = remote.NewTagger(options)
			}
		} else { // if  deps.Params.AgentType == NodeRemoteTaggerAgent
			options, _ := remote.NodeAgentOptions(deps.Config)
			defaultTagger = remote.NewTagger(options)
		}
	case LocalTagger:
		defaultTagger = local.NewTagger(deps.wmeta)
	case FakeTagger:
		// all binaries are expected to provide their own tagger at startup. we
		// provide a fake tagger for testing purposes, as calling the global
		// tagger without proper initialization is very common there.
		defaultTagger = local.NewFakeTagger()
	}
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
		defaultTagger.Start(deps.Context)
		return nil
	}})
	deps.Lc.Append(fx.Hook{OnStop: func(context.Context) error {
		defaultTagger.Stop()
		return nil
	}})
	return defaultTagger
}

// GetEntity returns the hash for the provided entity id.
func GetEntity(entityID string) (*types.Entity, error) {
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		entity, err := captureTagger.GetEntity(entityID)
		if err == nil && entity != nil {
			mux.RUnlock()
			return entity, nil
		}
	}
	mux.RUnlock()

	return defaultTagger.GetEntity(entityID)
}

// Tag queries the captureTagger (for replay scenarios) or the defaultTagger.
// It can return tags at high cardinality (with tags about individual containers),
// or at orchestrator cardinality (pod/task level).
func Tag(entity string, cardinality collectors.TagCardinality) ([]string, error) {
	// TODO: defer unlock once performance overhead of defer is negligible
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		tags, err := captureTagger.Tag(entity, cardinality)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	return defaultTagger.Tag(entity, cardinality)
}

// AccumulateTagsFor queries the defaultTagger to get entity tags from cache or
// sources and appends them to the TagsAccumulator.  It can return tags at high
// cardinality (with tags about individual containers), or at orchestrator
// cardinality (pod/task level).
func AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	// TODO: defer unlock once performance overhead of defer is negligible
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		err := captureTagger.AccumulateTagsFor(entity, cardinality, tb)
		if err == nil {
			mux.RUnlock()
			return nil
		}
	}
	mux.RUnlock()
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	return defaultTagger.AccumulateTagsFor(entity, cardinality, tb)
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func GetEntityHash(entity string, cardinality collectors.TagCardinality) string {
	tags, err := Tag(entity, cardinality)
	if err != nil {
		return ""
	}
	return utils.ComputeTagsHash(tags)
}

// StandardTags queries the defaultTagger to get entity
// standard tags (env, version, service) from cache or sources.
func StandardTags(entity string) ([]string, error) {
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		tags, err := captureTagger.Standard(entity)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	return defaultTagger.Standard(entity)
}

// AgentTags returns the agent tags
// It relies on the container provider utils to get the Agent container ID
func AgentTags(cardinality collectors.TagCardinality) ([]string, error) {
	ctrID, err := metrics.GetProvider().GetMetaCollector().GetSelfContainerID()
	if err != nil {
		return nil, err
	}

	if ctrID == "" {
		return nil, nil
	}

	entityID := containers.BuildTaggerEntityName(ctrID)
	return Tag(entityID, cardinality)
}

// GlobalTags queries global tags that should apply to all data coming from the
// agent.
func GlobalTags(cardinality collectors.TagCardinality) ([]string, error) {
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		tags, err := captureTagger.Tag(collectors.GlobalEntityID, cardinality)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	return defaultTagger.Tag(collectors.GlobalEntityID, cardinality)
}

// globalTagBuilder queries global tags that should apply to all data coming
// from the agent and appends them to the TagsAccumulator
func globalTagBuilder(cardinality collectors.TagCardinality, tb tagset.TagsAccumulator) error {
	mux.RLock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	if captureTagger != nil {
		err := captureTagger.AccumulateTagsFor(collectors.GlobalEntityID, cardinality, tb)

		if err == nil {
			mux.RUnlock()
			return nil
		}
	}
	mux.RUnlock()
	// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
	return defaultTagger.AccumulateTagsFor(collectors.GlobalEntityID, cardinality, tb)
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

// List the content of the defaulTagger
func List(cardinality collectors.TagCardinality) tagger_api.TaggerListResponse {
	return defaultTagger.List(cardinality)
}

// GetDefaultTagger returns the global Tagger instance
// TODO(components) (tagger): defaultTagger is a legacy global variable to be eliminated
func GetDefaultTagger() Component {
	return defaultTagger
}

// NewCaptureTagger sets the tagger to be used when replaying a capture
func NewCaptureTagger() Component {
	mux.Lock()
	defer mux.Unlock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	captureTagger = replay.NewTagger()
	return captureTagger
}

// ResetCaptureTagger resets the capture tagger to nil
func ResetCaptureTagger() {
	mux.Lock()
	defer mux.Unlock()
	// TODO(components) (tagger): captureTagger is a legacy global variable to be eliminated
	captureTagger = nil
}

// EnrichTags extends a tag list with origin detection tags
// NOTE(remy): it is not needed to sort/dedup the tags anymore since after the
// enrichment, the metric and its tags is sent to the context key generator, which
// is taking care of deduping the tags while generating the context key.
func EnrichTags(tb tagset.TagsAccumulator, udsOrigin string, clientOrigin string, cardinalityName string) {
	cardinality := taggerCardinality(cardinalityName)

	if udsOrigin != packets.NoOrigin {
		if err := AccumulateTagsFor(udsOrigin, cardinality, tb); err != nil {
			log.Errorf(err.Error())
		}
	}

	if err := globalTagBuilder(cardinality, tb); err != nil {
		log.Error(err.Error())
	}

	if clientOrigin != "" {
		if err := AccumulateTagsFor(clientOrigin, cardinality, tb); err != nil {
			tlmUDPOriginDetectionError.Inc()
			log.Tracef("Cannot get tags for entity %s: %s", clientOrigin, err)
		}
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
