package testutil

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSpan(t *testing.T) {
	for i, sc := range []SpanConfig{
		{1, 2},
		{0, 1},
		{},
		{5, 30},
		{0, 5},
		{5, 0},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			assert := assert.New(t)
			now := time.Now().UnixNano()
			s := GenerateSpan(&sc)
			assert.NotEmpty(s.Service)
			assert.NotEmpty(s.Name)
			assert.NotEmpty(s.Resource)
			assert.NotEmpty(s.Type)
			assert.NotZero(s.TraceID)
			assert.Equal(s.TraceID, s.SpanID)
			assert.Zero(s.ParentID)
			assert.NotZero(s.Duration)
			assert.InDelta(s.Start, now-s.Duration, 100000000)
			tagcount := len(s.Meta) + len(s.Metrics)
			assert.GreaterOrEqual(tagcount, sc.MinTags)
			if sc.MaxTags > 0 {
				assert.LessOrEqual(tagcount, sc.MaxTags)
			}
		})
	}
}
