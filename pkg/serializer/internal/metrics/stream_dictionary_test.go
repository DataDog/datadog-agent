// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// TestStreamDictionary_InternName_CachesAndAssignsMonotonicIDs verifies the
// core leaf-string interner contract: first call produces a new id + a
// MetricNameDefine datum; second call returns the cached id and appends no
// datum. Empty input returns the sentinel 0.
func TestStreamDictionary_InternName_CachesAndAssignsMonotonicIDs(t *testing.T) {
	d := NewStreamDictionary()
	var defs []*statefulpb.MetricDatum

	require.Equal(t, uint64(0), d.InternName("", &defs), "empty name returns sentinel 0")
	require.Empty(t, defs, "empty input emits no define")

	id1 := d.InternName("system.cpu.user", &defs)
	require.Equal(t, uint64(1), id1, "first name gets id=1")
	require.Len(t, defs, 1)
	require.Equal(t, uint64(1), defs[0].GetMetricNameDefine().Id)
	require.Equal(t, "system.cpu.user", defs[0].GetMetricNameDefine().Value)

	id2 := d.InternName("system.mem.used", &defs)
	require.Equal(t, uint64(2), id2)
	require.Len(t, defs, 2)

	// Re-intern the first name: should return cached id, no new define.
	idAgain := d.InternName("system.cpu.user", &defs)
	require.Equal(t, id1, idAgain)
	require.Len(t, defs, 2, "cached intern adds no defines")
}

// TestStreamDictionary_PersistsAcrossDefinesBatches verifies the dict
// retains its interner state across separate define-collection passes —
// this is the stream-scoped vs payload-scoped distinction.
func TestStreamDictionary_PersistsAcrossDefinesBatches(t *testing.T) {
	d := NewStreamDictionary()

	var firstBatch []*statefulpb.MetricDatum
	id := d.InternName("metric.a", &firstBatch)
	require.Equal(t, uint64(1), id)
	require.Len(t, firstBatch, 1)

	// Caller "flushes" the first batch.
	var secondBatch []*statefulpb.MetricDatum
	idAgain := d.InternName("metric.a", &secondBatch)
	require.Equal(t, id, idAgain, "id stable across flushes")
	require.Empty(t, secondBatch, "no new define on second batch — dict persists")
}

// TestStreamDictionary_InternTags_PrefixSharing verifies the v3 prefix-sharing
// scheme: when a CompositeTags has both t1 and t2 non-empty, t1 is interned
// as a standalone tagset, then t2 is interned with t1's id as prefix.
func TestStreamDictionary_InternTags_PrefixSharing(t *testing.T) {
	d := NewStreamDictionary()
	var defs []*statefulpb.MetricDatum

	// Two metrics sharing the same t1 (base tags) but different t2.
	t1 := []string{"env:prod", "host:web-1"}
	tagsA := tagset.NewCompositeTags(t1, []string{"service:api"})
	tagsB := tagset.NewCompositeTags(t1, []string{"service:api", "region:us-east"})

	idA := d.InternTags(tagsA, &defs)
	require.NotZero(t, idA)

	idB := d.InternTags(tagsB, &defs)
	require.NotZero(t, idB)
	require.NotEqual(t, idA, idB)

	// Find all tagset defines.
	var tagsetDefs []*statefulpb.MetricTagsetDefine
	for _, d := range defs {
		if td := d.GetMetricTagsetDefine(); td != nil {
			tagsetDefs = append(tagsetDefs, td)
		}
	}

	// Expect 3 tagsets: the shared base (prefix_id=0), tagsA (prefix_id=base),
	// tagsB (prefix_id=base).
	require.Len(t, tagsetDefs, 3)
	require.Equal(t, uint64(0), tagsetDefs[0].PrefixId, "base tagset has no prefix")
	require.NotZero(t, tagsetDefs[1].PrefixId, "tagsA references base as prefix")
	require.Equal(t, tagsetDefs[0].Id, tagsetDefs[1].PrefixId)
	require.Equal(t, tagsetDefs[0].Id, tagsetDefs[2].PrefixId, "tagsB also references base")

	// Re-interning either tagset returns the same id with no new defines.
	priorLen := len(defs)
	idAReinterned := d.InternTags(tagsA, &defs)
	require.Equal(t, idA, idAReinterned)
	require.Len(t, defs, priorLen, "cached tagset emits no new defines")
}

// TestStreamDictionary_InternTags_EmptyHalves verifies the edge cases:
// empty t1 + non-empty t2 (and vice versa) intern as standalone tagsets
// with prefix_id=0; both empty returns sentinel 0.
func TestStreamDictionary_InternTags_EmptyHalves(t *testing.T) {
	d := NewStreamDictionary()
	var defs []*statefulpb.MetricDatum

	empty := tagset.NewCompositeTags(nil, nil)
	require.Equal(t, uint64(0), d.InternTags(empty, &defs))
	require.Empty(t, defs)

	t2Only := tagset.NewCompositeTags(nil, []string{"only:tag"})
	idT2 := d.InternTags(t2Only, &defs)
	require.NotZero(t, idT2)
	// Should emit MetricTagStringDefine + MetricTagsetDefine (no prefix tagset).
	var hasTagset bool
	for _, d := range defs {
		if td := d.GetMetricTagsetDefine(); td != nil {
			require.Equal(t, uint64(0), td.PrefixId, "standalone tagset has prefix_id=0")
			hasTagset = true
		}
	}
	require.True(t, hasTagset)
}

// TestStreamDictionary_InternResources verifies the resource-set interner.
// A resource set is (Type, Name) pairs; both arrays must be emitted with
// equal length.
func TestStreamDictionary_InternResources(t *testing.T) {
	d := NewStreamDictionary()
	var defs []*statefulpb.MetricDatum

	require.Equal(t, uint64(0), d.InternResources(nil, &defs), "empty set returns sentinel 0")
	require.Empty(t, defs)

	res := []pkgmetrics.Resource{
		{Type: "host", Name: "agent-dev-docker"},
		{Type: "device", Name: "eth0"},
	}
	id := d.InternResources(res, &defs)
	require.NotZero(t, id)

	// Find the resource define. INVARIANT: len(type_string_ids) == len(name_string_ids).
	var found bool
	for _, d := range defs {
		if rd := d.GetMetricResourceDefine(); rd != nil {
			require.Len(t, rd.TypeStringIds, 2)
			require.Len(t, rd.NameStringIds, 2)
			require.Len(t, rd.TypeStringIds, len(rd.NameStringIds),
				"contract.md D6 invariant: parallel arrays equal length")
			found = true
		}
	}
	require.True(t, found, "expected MetricResourceDefine in emitted datums")

	// Re-intern returns cached id.
	priorLen := len(defs)
	require.Equal(t, id, d.InternResources(res, &defs))
	require.Len(t, defs, priorLen)
}

// TestStreamDictionary_InternOriginInfo verifies origin-tuple interning.
func TestStreamDictionary_InternOriginInfo(t *testing.T) {
	d := NewStreamDictionary()
	var defs []*statefulpb.MetricDatum

	o := originInfo{product: 10, category: 11, service: 38}
	id := d.InternOriginInfo(o, &defs)
	require.NotZero(t, id)
	require.Len(t, defs, 1)
	od := defs[0].GetMetricOriginDefine()
	require.NotNil(t, od)
	require.EqualValues(t, 10, od.Product)
	require.EqualValues(t, 11, od.Category)
	require.EqualValues(t, 38, od.Service)

	// Same tuple → cached.
	require.Equal(t, id, d.InternOriginInfo(o, &defs))
	require.Len(t, defs, 1)
}
