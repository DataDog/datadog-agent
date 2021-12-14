// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagstore

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// sourceTags holds the tags for a given entity collected from a single source,
// grouped by their cardinality.
type sourceTags struct {
	entityID           string
	cachedAll          tagset.HashedTags // Low + orchestrator + high
	cachedOrchestrator tagset.HashedTags // Low + orchestrator (subslice of cachedAll)
	cachedLow          tagset.HashedTags // Sub-slice of cachedAll
	standardTags       []string
	expiryDate         time.Time
}

func newSourceTags(info *collectors.TagInfo) sourceTags {
	tags := append(info.LowCardTags, info.OrchestratorCardTags...)
	tags = append(tags, info.HighCardTags...)

	cached := tagset.NewHashedTagsFromSlice(tags)

	return sourceTags{
		entityID:           info.Entity,
		cachedAll:          cached,
		cachedLow:          cached.Slice(0, len(info.LowCardTags)),
		cachedOrchestrator: cached.Slice(0, len(info.LowCardTags)+len(info.OrchestratorCardTags)),
		standardTags:       info.StandardTags,
		expiryDate:         info.ExpiryDate,
	}
}

func (st *sourceTags) isExpired(t time.Time) bool {
	if st.expiryDate.IsZero() {
		return false
	}

	return st.expiryDate.Before(t)
}

func (st *sourceTags) toEntity() types.Entity {
	all := st.cachedAll.Get()
	low := st.cachedLow.Len()
	orch := st.cachedOrchestrator.Len()
	return types.Entity{
		ID:                          st.entityID,
		StandardTags:                st.standardTags,
		HighCardinalityTags:         all[orch:],
		OrchestratorCardinalityTags: all[low:orch],
		LowCardinalityTags:          all[:low],
	}
}

func (st *sourceTags) getHashedTags(cardinality collectors.TagCardinality) tagset.HashedTags {
	if cardinality == collectors.HighCardinality {
		return st.cachedAll
	} else if cardinality == collectors.OrchestratorCardinality {
		return st.cachedOrchestrator
	}
	return st.cachedLow
}
