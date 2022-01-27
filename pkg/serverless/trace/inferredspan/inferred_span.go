package inferredspan

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// tagInferredSpanTagSource is the key to the meta tag
	// that lets us know whether this span should inherit its tags.
	// Expected options are "lambda" and "self"
	tagInferredSpanTagSource = "_inferred_span.tag_source"
	functionVersionTagKey    = "function_version"
	coldStartTagKey          = "cold_start"
)

var globalTagsToFilter map[string]bool
var functionTagsToIgnore = [...]string{
	tags.FunctionARNKey,
	tags.FunctionNameKey,
	tags.ExecutedVersionKey,
	tags.EnvKey,
	tags.VersionKey,
	tags.ServiceKey,
	tags.RuntimeKey,
	tags.MemorySizeKey,
	tags.ArchitectureKey,
	functionVersionTagKey,
	coldStartTagKey,
}

// Check determines if a span is an inferred span or not.
func Check(span *pb.Span) bool {
	if _, ok := span.Meta[tagInferredSpanTagSource]; ok {
		return true
	}
	return false
}

// FilterFunctionTags filters out function specific tags
func FilterFunctionTags(span *pb.Span, input *map[string]string) {
	ddTags := config.GetConfiguredTags(false)

	for _, tag := range ddTags {
		tagParts := strings.SplitN(tag, ":", 2)
		if len(tagParts) != 2 {
			log.Warnf("Cannot split tag %s", tag)
			continue
		}
		tagKey := tagParts[0]
		delete(*input, tagKey)
		delete(*input, "sometag")
	}

	for _, tagKey := range functionTagsToIgnore {
		delete(*input, tagKey)
	}
}
