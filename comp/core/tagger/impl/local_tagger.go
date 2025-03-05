// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package taggerimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tagstore"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	// pidCacheTTL is the time to live for the PID cache
	pidCacheTTL = 1 * time.Second
	// inodeCacheTTL is the time to live for the inode cache
	inodeCacheTTL = 1 * time.Second
	// externalDataCacheTTL is the time to live for the external data cache
	externalDataCacheTTL = 1 * time.Second
)

// Tagger is the entry class for entity tagging. It hold the tagger collector,
// memory store, and handles the query logic. One should use the package
// methods in comp/core/tagger to use the default Tagger instead of instantiating it
// directly.
type localTagger struct {
	sync.RWMutex

	tagStore      *tagstore.TagStore
	workloadStore workloadmeta.Component
	log           log.Component
	cfg           config.Component
	collector     *collectors.WorkloadMetaCollector

	ctx            context.Context
	cancel         context.CancelFunc
	telemetryStore *telemetry.Store
}

func newLocalTagger(cfg config.Component, wmeta workloadmeta.Component, log log.Component, telemetryStore *telemetry.Store) (tagger.Component, error) {
	return &localTagger{
		tagStore:       tagstore.NewTagStore(telemetryStore),
		workloadStore:  wmeta,
		log:            log,
		telemetryStore: telemetryStore,
		cfg:            cfg,
	}, nil
}

// Start starts the workloadmeta collector and then it is ready for requests.
func (t *localTagger) Start(ctx context.Context) error {
	t.ctx, t.cancel = context.WithCancel(ctx)

	t.collector = collectors.NewWorkloadMetaCollector(
		t.ctx,
		t.cfg,
		t.workloadStore,
		t.tagStore,
	)

	go t.tagStore.Run(t.ctx)
	go t.collector.Run(t.ctx, t.cfg)

	return nil
}

// Stop queues a shutdown of Tagger
func (t *localTagger) Stop() error {
	t.cancel()
	return nil
}

// getTags returns a read only list of tags for a given entity.
func (t *localTagger) getTags(entityID types.EntityID, cardinality types.TagCardinality) (tagset.HashedTags, error) {
	if entityID.Empty() {
		t.telemetryStore.QueriesByCardinality(cardinality).EmptyEntityID.Inc()
		return tagset.HashedTags{}, fmt.Errorf("empty entity ID")
	}

	cachedTags := t.tagStore.LookupHashedWithEntityStr(entityID, cardinality)

	t.telemetryStore.QueriesByCardinality(cardinality).Success.Inc()
	return cachedTags, nil
}

// AccumulateTagsFor appends tags for a given entity from the tagger to the TagsAccumulator
func (t *localTagger) AccumulateTagsFor(entityID types.EntityID, cardinality types.TagCardinality, tb tagset.TagsAccumulator) error {
	tags, err := t.getTags(entityID, cardinality)
	tb.AppendHashed(tags)
	return err
}

// Tag returns a copy of the tags for a given entity
func (t *localTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	tags, err := t.getTags(entityID, cardinality)
	if err != nil {
		return nil, err
	}
	return tags.Copy(), nil
}

// GenerateContainerIDFromOriginInfo generates a container ID from Origin Info.
// The resolutions will be done in the following order:
// * OriginInfo.LocalData.ContainerID: If the container ID is already known, return it.
// * OriginInfo.LocalData.ProcessID: If the process ID is known, do a PID resolution.
// * OriginInfo.LocalData.Inode: If the inode is known, do an inode resolution.
// * OriginInfo.ExternalData: If the ExternalData are known, do an ExternalData resolution.
func (t *localTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (containerID string, err error) {
	t.log.Debugf("Generating container ID from OriginInfo: %+v", originInfo)
	// If the container ID is already known, return it.
	if originInfo.LocalData.ContainerID != "" {
		t.log.Debugf("Found OriginInfo.LocalData.ContainerID: %s", originInfo.LocalData.ContainerID)
		containerID = originInfo.LocalData.ContainerID
		return
	}

	// Get the MetaCollector from WorkloadMeta.
	metaCollector := metrics.GetProvider(option.New(t.workloadStore)).GetMetaCollector()

	// If the process ID is known, do a PID resolution.
	if originInfo.LocalData.ProcessID != 0 {
		t.log.Debugf("Resolving container ID from PID: %d", originInfo.LocalData.ProcessID)
		containerID, err = metaCollector.GetContainerIDForPID(int(originInfo.LocalData.ProcessID), pidCacheTTL)
		if err != nil {
			t.log.Debugf("Error resolving container ID from PID: %v", err)
		} else if containerID == "" {
			t.log.Debugf("No container ID found for PID: %d", originInfo.LocalData.ProcessID)
		} else {
			return
		}
	}

	// If the inode is known, do an inode resolution.
	if originInfo.LocalData.Inode != 0 {
		t.log.Debugf("Resolving container ID from inode: %d", originInfo.LocalData.Inode)
		containerID, err = metaCollector.GetContainerIDForInode(originInfo.LocalData.Inode, inodeCacheTTL)
		if err != nil {
			t.log.Debugf("Error resolving container ID from inode: %v", err)
		} else if containerID == "" {
			t.log.Debugf("No container ID found for inode: %d", originInfo.LocalData.Inode)
		} else {
			return
		}
	}

	// If the ExternalData are known, do an ExternalData resolution.
	if originInfo.ExternalData.PodUID != "" && originInfo.ExternalData.ContainerName != "" {
		t.log.Debugf("Resolving container ID from ExternalData: %+v", originInfo.ExternalData)
		containerID, err = metaCollector.ContainerIDForPodUIDAndContName(originInfo.ExternalData.PodUID, originInfo.ExternalData.ContainerName, originInfo.ExternalData.Init, externalDataCacheTTL)
		if err != nil {
			t.log.Debugf("Error resolving container ID from ExternalData: %v", err)
		} else if containerID == "" {
			t.log.Debugf("No container ID found for ExternalData: %+v", originInfo.ExternalData)
		} else {
			return
		}
	}

	return "", fmt.Errorf("unable to resolve container ID from OriginInfo: %+v", originInfo)
}

// LegacyTag has the same behaviour as the Tag method, but it receives the entity id as a string and parses it.
// If possible, avoid using this function, and use the Tag method instead.
// This function exists in order not to break backward compatibility with rtloader and python
// integrations using the tagger
func (t *localTagger) LegacyTag(entity string, cardinality types.TagCardinality) ([]string, error) {
	prefix, id, err := types.ExtractPrefixAndID(entity)
	if err != nil {
		return nil, err
	}

	entityID := types.NewEntityID(prefix, id)
	return t.Tag(entityID, cardinality)
}

// Standard returns standard tags for a given entity
// It triggers a tagger fetch if the no tags are found
func (t *localTagger) Standard(entityID types.EntityID) ([]string, error) {
	if entityID.Empty() {
		return nil, fmt.Errorf("empty entity ID")
	}

	return t.tagStore.LookupStandard(entityID)
}

// GetEntity returns the entity corresponding to the specified id and an error
func (t *localTagger) GetEntity(entityID types.EntityID) (*types.Entity, error) {
	return t.tagStore.GetEntity(entityID)
}

// List the content of the tagger
func (t *localTagger) List() types.TaggerListResponse {
	return t.tagStore.List()
}

// Subscribe returns a channel that receives a slice of events whenever an entity is
// added, modified or deleted. It can send an initial burst of events only to the new
// subscriber, without notifying all of the others.
func (t *localTagger) Subscribe(subscriptionID string, filter *types.Filter) (types.Subscription, error) {
	return t.tagStore.Subscribe(subscriptionID, filter)
}

// GetTaggerTelemetryStore returns tagger telemetry store
func (t *localTagger) GetTaggerTelemetryStore() *telemetry.Store {
	return t.telemetryStore
}

func (t *localTagger) GetEntityHash(types.EntityID, types.TagCardinality) string {
	return ""
}

func (t *localTagger) AgentTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *localTagger) GlobalTags(types.TagCardinality) ([]string, error) {
	return []string{}, nil
}

func (t *localTagger) EnrichTags(tagset.TagsAccumulator, taggertypes.OriginInfo) {}

func (t *localTagger) ChecksCardinality() types.TagCardinality {
	return types.LowCardinality
}
