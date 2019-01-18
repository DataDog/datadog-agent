package agent

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestGrain(t *testing.T) {
	srb := NewStatsRawBucket(0, 1e9)
	assert := assert.New(t)

	s := pb.Span{Service: "thing", Name: "other", Resource: "yo"}
	aggr, tgs := assembleGrain(&srb.keyBuf, "default", s.Resource, s.Service, nil)

	assert.Equal("env:default,resource:yo,service:thing", aggr)
	assert.Equal(TagSet{Tag{"env", "default"}, Tag{"resource", "yo"}, Tag{"service", "thing"}}, tgs)
}

func TestGrainWithExtraTags(t *testing.T) {
	srb := NewStatsRawBucket(0, 1e9)
	assert := assert.New(t)

	s := pb.Span{Service: "thing", Name: "other", Resource: "yo", Meta: map[string]string{"meta2": "two", "meta1": "ONE"}}
	aggr, tgs := assembleGrain(&srb.keyBuf, "default", s.Resource, s.Service, s.Meta)

	assert.Equal("env:default,resource:yo,service:thing,meta1:ONE,meta2:two", aggr)
	assert.Equal(TagSet{Tag{"env", "default"}, Tag{"resource", "yo"}, Tag{"service", "thing"}, Tag{"meta1", "ONE"}, Tag{"meta2", "two"}}, tgs)
}
