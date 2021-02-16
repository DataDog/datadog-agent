package tagger

import (
	"github.com/DataDog/datadog-agent/cmd/agent/api/response"
	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
	"github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/util"
)

// Tagger is an interface for transparent access to both localTagger and
// remoteTagger.
type Tagger interface {
	Init() error
	Stop() error

	Tag(entity string, cardinality collectors.TagCardinality) ([]string, error)
	TagBuilder(entity string, cardinality collectors.TagCardinality, tb *util.TagsBuilder) error
	Standard(entity string) ([]string, error)
	List(cardinality collectors.TagCardinality) response.TaggerListResponse

	Subscribe(cardinality collectors.TagCardinality) chan []types.EntityEvent
	Unsubscribe(ch chan []types.EntityEvent)
}
