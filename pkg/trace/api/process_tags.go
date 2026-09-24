// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"strings"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

// highCardinalityProcessTags are process tag keys sent by tracers whose values
// are not stable across deployments or instances of the same service (e.g.
// versioned release directories, temporary directories). Process tags are part
// of the stats aggregation key, so these are dropped on intake.
var highCardinalityProcessTags = map[string]struct{}{
	"entrypoint.workdir": {},
	"entrypoint.basedir": {},
}

func isHighCardinalityProcessTag(tag string) bool {
	key, _, _ := strings.Cut(tag, ":")
	_, ok := highCardinalityProcessTags[strings.TrimSpace(key)]
	return ok
}

// filterProcessTags removes high cardinality tags from a comma-separated list
// of process tags. The input is returned as-is when there is nothing to remove.
func filterProcessTags(ptags string) string {
	if ptags == "" {
		return ""
	}
	tags := strings.Split(ptags, ",")
	kept := tags[:0]
	for _, tag := range tags {
		if !isHighCardinalityProcessTag(tag) {
			kept = append(kept, tag)
		}
	}
	if len(kept) == len(tags) {
		return ptags
	}
	return strings.Join(kept, ",")
}

// filterSpanProcessTags filters the process tags carried in the meta of the
// first span of each chunk, so that they don't reach the backend unfiltered.
func filterSpanProcessTags(p *pb.TracerPayload) {
	for _, chunk := range p.Chunks {
		if len(chunk.Spans) == 0 || chunk.Spans[0] == nil {
			continue
		}
		meta := chunk.Spans[0].Meta
		ptags, ok := meta[tagProcessTags]
		if !ok {
			continue
		}
		if filtered := filterProcessTags(ptags); filtered == "" {
			delete(meta, tagProcessTags)
		} else {
			meta[tagProcessTags] = filtered
		}
	}
}

// filterSpanProcessTagsV1 is the equivalent of filterSpanProcessTags for v1 payloads.
func filterSpanProcessTagsV1(p *idx.InternalTracerPayload) {
	for _, chunk := range p.Chunks {
		if len(chunk.Spans) == 0 || chunk.Spans[0] == nil {
			continue
		}
		span := chunk.Spans[0]
		ptags, ok := span.GetAttributeAsString(tagProcessTags)
		if !ok {
			continue
		}
		if filtered := filterProcessTags(ptags); filtered == "" {
			span.DeleteAttribute(tagProcessTags)
		} else if filtered != ptags {
			span.SetStringAttribute(tagProcessTags, filtered)
		}
	}
}
