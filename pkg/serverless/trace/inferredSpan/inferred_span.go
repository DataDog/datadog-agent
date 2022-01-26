package inferredSpan

import (
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"os"
	"strings"
)

const (
	// tagInferredSpanTagSource is the key to the meta tag that lets us know whether this span should inherit its tags.
	// Expected options are "lambda" and "self"
	tagInferredSpanTagSource = "_inferred_span.tag_source"
)

var globalTagsToFilter map[string]struct{}

func populateGlobalTagsToFilter() {
	if globalTagsToFilter != nil {
		return
	}
	globalTagsToFilter = make(map[string]struct{})
	ddTagsStr := os.Getenv("DD_TAGS")
	separator := " "
	if strings.Contains(ddTagsStr, ",") {
		separator = ","
	}
	rawDdTags := strings.Split(ddTagsStr, separator)
	for _, tag := range rawDdTags {
		colonIdx := strings.Index(tag, ":")
		if colonIdx != 0 {
			tagKey := tag[:colonIdx]
			globalTagsToFilter[tagKey] = struct{}{}
		}
	}
}

// Check determines if a span is an inferred span or not. Returns true if so, false otherwise.
func Check(span *pb.Span) bool {
	if _, ok := span.Meta[tagInferredSpanTagSource]; ok {
		return true
	}
	return false
}

// FilterTags takes a map of tags and filters out the tags that we do not want to set on an inferred span.
// It does this by lazily initializing a singleton set of tags to filter out, and then re-uses this set in future invocations.
func FilterTags(span *pb.Span, input map[string]string) map[string]string {
	isInferred := Check(span)
	if !isInferred {
		return input
	}

	if span.Meta[tagInferredSpanTagSource] == "lambda" {
		return input
	}

	output := make(map[string]string)
	populateGlobalTagsToFilter()

	for k, v := range input {
		_, filterOut := globalTagsToFilter[k]
		if !filterOut {
			output[k] = v
		}
	}
	return output
}
