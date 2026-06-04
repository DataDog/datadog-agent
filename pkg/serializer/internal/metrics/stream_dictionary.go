// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"slices"

	"github.com/twmb/murmur3"

	pkgmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// StreamDictionary is the agent-side, stream-scoped, monotonic-ID interner
// for the stateful v3 metrics path. It mirrors the role of the v3
// dictionaryBuilder (which is payload-scoped) — same interner kinds, same
// prefix-sharing for tagsets, same murmur3-keyed lookups — but persists
// across payloads so the dictionary accumulates rather than re-bootstrapping
// per flush.
//
// Lifecycle ownership:
//   - One StreamDictionary per metric stateful lane (PoC: one lane → one dict).
//   - Lives in the serializer (held alongside the gRPC Sender). Not reset on
//     stream rotation — IDs assigned here are stable for the lane's lifetime.
//     The streamWorker handles rotation by retransmitting buffered payloads
//     and emitting batch_id=0 from the inflight tracker's snapshot.
//   - All methods are single-goroutine (the encoder writes; nothing else
//     touches the dict). No mutex.
//
// IDs are uint64, monotonic from 1. Id=0 is the "absent" sentinel
// (matches v3 semantics; e.g. an empty metric name produces ref=0).
// Each interner kind has its own ID space.
//
// All Intern* methods take a `defines` pointer-to-slice. When a new entry
// is created, the corresponding MetricXxxDefine datum is appended to that
// slice — the encoder is responsible for emitting those defines on the
// wire before any ref column references them. Cached lookups append
// nothing (the receiver already knows the id).
type StreamDictionary struct {
	// Leaf string interners.
	names      map[string]uint64
	nextNameID uint64

	tagStrings      map[string]uint64
	nextTagStringID uint64

	sourceTypeNames      map[string]uint64
	nextSourceTypeNameID uint64

	resourceStrings      map[string]uint64
	nextResourceStringID uint64

	// Origin tuple interner.
	origins      map[originInfo]uint64
	nextOriginID uint64

	// Composite interners (key by murmur3 hash of constituent parts).
	tagsetIndex  map[uint64]uint64
	nextTagsetID uint64

	resourcesIndex map[uint64]uint64
	nextResourceID uint64

	// Scratch buffers (avoid per-call allocation in hot path).
	tagsStringBuf []string
}

// NewStreamDictionary constructs an empty dictionary.
func NewStreamDictionary() *StreamDictionary {
	return &StreamDictionary{
		names:           make(map[string]uint64),
		tagStrings:      make(map[string]uint64),
		sourceTypeNames: make(map[string]uint64),
		resourceStrings: make(map[string]uint64),
		origins:         make(map[originInfo]uint64),
		tagsetIndex:     make(map[uint64]uint64),
		resourcesIndex:  make(map[uint64]uint64),
	}
}

// --- Leaf string interners ---------------------------------------------

func (d *StreamDictionary) InternName(name string, defines *[]*statefulpb.MetricDatum) uint64 {
	if name == "" {
		return 0
	}
	if id, ok := d.names[name]; ok {
		return id
	}
	d.nextNameID++
	d.names[name] = d.nextNameID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricNameDefine{
			MetricNameDefine: &statefulpb.MetricNameDefine{Id: d.nextNameID, Value: name},
		},
	})
	return d.nextNameID
}

// internTagString is the unexported helper used by InternTags. Same shape
// as InternName but for the tag-string kind.
func (d *StreamDictionary) internTagString(s string, defines *[]*statefulpb.MetricDatum) uint64 {
	if id, ok := d.tagStrings[s]; ok {
		return id
	}
	d.nextTagStringID++
	d.tagStrings[s] = d.nextTagStringID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricTagStringDefine{
			MetricTagStringDefine: &statefulpb.MetricTagStringDefine{Id: d.nextTagStringID, Value: s},
		},
	})
	return d.nextTagStringID
}

func (d *StreamDictionary) InternSourceTypeName(stn string, defines *[]*statefulpb.MetricDatum) uint64 {
	if stn == "" {
		return 0
	}
	if id, ok := d.sourceTypeNames[stn]; ok {
		return id
	}
	d.nextSourceTypeNameID++
	d.sourceTypeNames[stn] = d.nextSourceTypeNameID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricSourceTypeNameDefine{
			MetricSourceTypeNameDefine: &statefulpb.MetricSourceTypeNameDefine{Id: d.nextSourceTypeNameID, Value: stn},
		},
	})
	return d.nextSourceTypeNameID
}

// internResourceString is the unexported helper used by InternResources.
func (d *StreamDictionary) internResourceString(s string, defines *[]*statefulpb.MetricDatum) uint64 {
	if id, ok := d.resourceStrings[s]; ok {
		return id
	}
	d.nextResourceStringID++
	d.resourceStrings[s] = d.nextResourceStringID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricResourceStringDefine{
			MetricResourceStringDefine: &statefulpb.MetricResourceStringDefine{Id: d.nextResourceStringID, Value: s},
		},
	})
	return d.nextResourceStringID
}

// --- Origin tuple interner ---------------------------------------------

func (d *StreamDictionary) InternOriginInfo(info originInfo, defines *[]*statefulpb.MetricDatum) uint64 {
	if id, ok := d.origins[info]; ok {
		return id
	}
	d.nextOriginID++
	d.origins[info] = d.nextOriginID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricOriginDefine{
			MetricOriginDefine: &statefulpb.MetricOriginDefine{
				Id:       d.nextOriginID,
				Product:  info.product,
				Category: info.category,
				Service:  info.service,
			},
		},
	})
	return d.nextOriginID
}

// --- Composite: resources ----------------------------------------------

// InternResources interns a resource set. Each resource is a (Type, Name)
// pair; the set is the per-series ordered list. Returns the resource-set
// id (0 if the set is empty).
//
// Side effect: any newly-introduced resource strings are emitted as
// MetricResourceStringDefine datums; the new resource-set itself is
// emitted as MetricResourceDefine.
func (d *StreamDictionary) InternResources(res []pkgmetrics.Resource, defines *[]*statefulpb.MetricDatum) uint64 {
	if len(res) == 0 {
		return 0
	}

	// Hash key: murmur3 over the (Type, Name) sequence.
	var hash1, hash2 uint64
	for _, r := range res {
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, r.Type)
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, r.Name)
	}
	if id, ok := d.resourcesIndex[hash1]; ok {
		return id
	}

	// First: intern each constituent string. This may emit
	// MetricResourceStringDefine datums.
	typeStringIDs := make([]uint64, len(res))
	nameStringIDs := make([]uint64, len(res))
	for i, r := range res {
		typeStringIDs[i] = d.internResourceString(r.Type, defines)
		nameStringIDs[i] = d.internResourceString(r.Name, defines)
	}

	// Then: define the resource-set itself.
	d.nextResourceID++
	d.resourcesIndex[hash1] = d.nextResourceID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricResourceDefine{
			MetricResourceDefine: &statefulpb.MetricResourceDefine{
				Id:            d.nextResourceID,
				TypeStringIds: typeStringIDs,
				NameStringIds: nameStringIDs,
			},
		},
	})
	return d.nextResourceID
}

// --- Composite: tagsets with prefix-sharing ---------------------------

// InternTags interns a CompositeTags (typically t1 = base tags, t2 = per-metric
// tags) using the v3 prefix-sharing scheme:
//   - empty t1 + empty t2 → return 0
//   - either t1 or t2 empty → intern the non-empty half standalone (prefix_id=0)
//   - both non-empty → intern t1 as a standalone tagset, then intern t2
//     with prefix_id = t1's tagset id
//
// Side effect: any newly-introduced tag strings are emitted as
// MetricTagStringDefine datums; new tagsets are emitted as MetricTagsetDefine.
// Order of emission satisfies the wire invariant "defines precede references":
// tag strings first (they're referenced by the tagset), then the tagset.
//
// The cross-emission order between the prefix tagset and the suffix tagset is
// also correct: the prefix is interned first (and any new tag strings + the
// prefix-tagset-define are emitted), then the suffix (referencing the prefix id
// already emitted earlier in the defines slice).
func (d *StreamDictionary) InternTags(tags tagset.CompositeTags, defines *[]*statefulpb.MetricDatum) uint64 {
	t1, t2 := tags.UnsafeGet()
	switch {
	case len(t1) == 0 && len(t2) == 0:
		return 0
	case len(t1) == 0:
		return d.internTagset1(0, t2, defines)
	case len(t2) == 0:
		return d.internTagset1(0, t1, defines)
	default:
		prefixID := d.internTagset1(0, t1, defines)
		return d.internTagset1(prefixID, t2, defines)
	}
}

// internTagset1 interns one half of a CompositeTags tagset. Returns the
// tagset id. If prefix_id is non-zero, this tagset is "prefix's tags ∪
// these tags."
func (d *StreamDictionary) internTagset1(prefixID uint64, tags []string, defines *[]*statefulpb.MetricDatum) uint64 {
	// Sort tags for stable hashing (v3 invariant). Use the scratch buffer
	// to avoid allocating per call.
	defer func() { d.tagsStringBuf = d.tagsStringBuf[:0] }()
	d.tagsStringBuf = append(d.tagsStringBuf, tags...)
	slices.Sort(d.tagsStringBuf)

	var hash1, hash2 uint64 = prefixID, 0
	for _, s := range d.tagsStringBuf {
		hash1, hash2 = murmur3.SeedStringSum128(hash1, hash2, s)
	}
	if id, ok := d.tagsetIndex[hash1]; ok {
		return id
	}

	// First: intern each constituent tag string. This may emit
	// MetricTagStringDefine datums.
	tagStringIDs := make([]uint64, len(d.tagsStringBuf))
	for i, s := range d.tagsStringBuf {
		tagStringIDs[i] = d.internTagString(s, defines)
	}

	// Then: define the tagset itself.
	d.nextTagsetID++
	d.tagsetIndex[hash1] = d.nextTagsetID
	*defines = append(*defines, &statefulpb.MetricDatum{
		Data: &statefulpb.MetricDatum_MetricTagsetDefine{
			MetricTagsetDefine: &statefulpb.MetricTagsetDefine{
				Id:           d.nextTagsetID,
				PrefixId:     prefixID,
				TagStringIds: tagStringIDs,
			},
		},
	})
	return d.nextTagsetID
}
