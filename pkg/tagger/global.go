// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagger

import (
	"sync"

	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/local"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// defaultTagger is the shared tagger instance backing the global Tag and Init functions
var defaultTagger Tagger
var initOnce sync.Once
var initErr error

// captureTagger is a tagger instance that contains a tagger that will contain the tagger
// state when replaying a capture scenario
var captureTagger Tagger
var mux sync.RWMutex

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

// Init must be called once config is available, call it in your cmd
func Init() error {
	initOnce.Do(func() {
		var err error
		checkCard := config.Datadog.GetString("checks_tag_cardinality")
		dsdCard := config.Datadog.GetString("dogstatsd_tag_cardinality")

		ChecksCardinality, err = collectors.StringToTagCardinality(checkCard)
		if err != nil {
			log.Warnf("failed to parse check tag cardinality, defaulting to low. Error: %s", err)
			ChecksCardinality = collectors.LowCardinality
		}

		DogstatsdCardinality, err = collectors.StringToTagCardinality(dsdCard)
		if err != nil {
			log.Warnf("failed to parse dogstatsd tag cardinality, defaulting to low. Error: %s", err)
			DogstatsdCardinality = collectors.LowCardinality
		}

		if config.IsCLCRunner() {
			log.Infof("Tagger not started on CLC")
			return
		}

		initErr = defaultTagger.Init()
	})

	return initErr
}

// GetEntity returns the hash for the provided entity id.
func GetEntity(entityID string) (*types.Entity, error) {
	mux.RLock()
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
	//TODO: defer unlock once performance overhead of defer is negligible
	mux.RLock()
	if captureTagger != nil {
		tags, err := captureTagger.Tag(entity, cardinality)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()

	return defaultTagger.Tag(entity, cardinality)
}

// AccumulateTagsFor queries the defaultTagger to get entity tags from cache or
// sources and appends them to the TagAccumulator.  It can return tags at high
// cardinality (with tags about individual containers), or at orchestrator
// cardinality (pod/task level).
func AccumulateTagsFor(entity string, cardinality collectors.TagCardinality, tb tagset.TagAccumulator) error {
	//TODO: defer unlock once performance overhead of defer is negligible
	mux.RLock()
	if captureTagger != nil {
		err := captureTagger.AccumulateTagsFor(entity, cardinality, tb)
		if err == nil {
			mux.RUnlock()
			return nil
		}
	}
	mux.RUnlock()

	return defaultTagger.AccumulateTagsFor(entity, cardinality, tb)
}

// TagWithHash is similar to Tag but it also computes and returns the hash of the tags found
func TagWithHash(entity string, cardinality collectors.TagCardinality) ([]string, string, error) {

	tags, err := Tag(entity, cardinality)
	if err != nil {
		return tags, "", err
	}
	return tags, utils.ComputeTagsHash(tags), nil
}

// GetEntityHash returns the hash for the tags associated with the given entity
// Returns an empty string if the tags lookup fails
func GetEntityHash(entity string, cardinality collectors.TagCardinality) string {
	_, hash, _ := TagWithHash(entity, cardinality)
	return hash
}

// StandardTags queries the defaultTagger to get entity
// standard tags (env, version, service) from cache or sources.
func StandardTags(entity string) ([]string, error) {
	mux.RLock()
	if captureTagger != nil {
		tags, err := captureTagger.Standard(entity)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()

	return defaultTagger.Standard(entity)
}

// AgentTags returns the agent tags
// It relies on the container provider utils to get the Agent container ID
func AgentTags(cardinality collectors.TagCardinality) ([]string, error) {

	ctrID, err := providers.ContainerImpl().GetAgentCID()
	if err != nil {
		return nil, err
	}

	entityID := containers.BuildTaggerEntityName(ctrID)
	return Tag(entityID, cardinality)
}

// OrchestratorScopeTag queries tags for orchestrator scope (e.g. task_arn in ECS Fargate)
func OrchestratorScopeTag() ([]string, error) {
	mux.RLock()
	if captureTagger != nil {
		tags, err := captureTagger.Tag(collectors.OrchestratorScopeEntityID, collectors.OrchestratorCardinality)
		if err == nil && len(tags) > 0 {
			mux.RUnlock()
			return tags, nil
		}
	}
	mux.RUnlock()

	return defaultTagger.Tag(collectors.OrchestratorScopeEntityID, collectors.OrchestratorCardinality)
}

// OrchestratorScopeTagBuilder queries tags for orchestrator scope (e.g.
// task_arn in ECS Fargate) and appends them to the TagAccumulator
func OrchestratorScopeTagBuilder(tb tagset.TagAccumulator) error {
	mux.RLock()
	if captureTagger != nil {
		err := captureTagger.AccumulateTagsFor(collectors.OrchestratorScopeEntityID, collectors.OrchestratorCardinality, tb)

		if err == nil {
			mux.RUnlock()
			return nil
		}
	}
	mux.RUnlock()

	return defaultTagger.AccumulateTagsFor(collectors.OrchestratorScopeEntityID, collectors.OrchestratorCardinality, tb)
}

// Stop queues a stop signal to the defaultTagger
func Stop() error {
	return defaultTagger.Stop()
}

// List the content of the defaulTagger
func List(cardinality collectors.TagCardinality) response.TaggerListResponse {
	return defaultTagger.List(cardinality)
}

// SetDefaultTagger sets the global Tagger instance
func SetDefaultTagger(tagger Tagger) {
	defaultTagger = tagger
}

// GetDefaultTagger returns the global Tagger instance
func GetDefaultTagger() Tagger {
	return defaultTagger
}

// SetCaptureTagger sets the tagger to be used when replaying a capture
func SetCaptureTagger(tagger Tagger) {
	mux.Lock()
	defer mux.Unlock()

	captureTagger = tagger
}

// ResetCaptureTagger resets the capture tagger to nil
func ResetCaptureTagger() {
	mux.Lock()
	defer mux.Unlock()

	captureTagger = nil
}

func init() {
	SetDefaultTagger(local.NewTagger(collectors.DefaultCatalog))
}

// EnrichTags extends a tag list with origin detection tags
// NOTE(remy): it is not needed to sort/dedup the tags anymore since after the
// enrichment, the metric and its tags is sent to the context key generator, which
// is taking care of deduping the tags while generating the context key.
func EnrichTags(tb tagset.TagAccumulator, origin string, k8sOriginID string, cardinalityName string) {
	cardinality := taggerCardinality(cardinalityName)

	if origin != packets.NoOrigin {
		if err := AccumulateTagsFor(origin, cardinality, tb); err != nil {
			log.Errorf(err.Error())
		}
	}

	// Include orchestrator scope tags if the cardinality is set to orchestrator
	if cardinality == collectors.OrchestratorCardinality {
		if err := OrchestratorScopeTagBuilder(tb); err != nil {
			log.Error(err.Error())
		}
	}

	if k8sOriginID != "" {
		if err := AccumulateTagsFor(k8sOriginID, cardinality, tb); err != nil {
			tlmUDPOriginDetectionError.Inc()
			log.Tracef("Cannot get tags for entity %s: %s", k8sOriginID, err)
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
		log.Tracef("Couldn't convert cardinality tag: %w", err)
		return DogstatsdCardinality
	}

	return taggerCardinality
}
