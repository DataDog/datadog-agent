package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func testSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   42,
		Start:    1448466874000000000,
		TraceID:  424242,
		Meta: map[string]string{
			"user": "leo",
			"pool": "fondue",
		},
		Metrics: map[string]float64{
			"cheese_weight": 100000.0,
		},
		ParentID: 1111,
		Type:     "http",
	}
}

func TestNormalizeOK(t *testing.T) {
	s := testSpan()
	assert.NoError(t, Normalize(s))
}

func TestNormalizeServicePassThru(t *testing.T) {
	s := testSpan()
	before := s.Service
	Normalize(s)
	assert.Equal(t, before, s.Service)
}

func TestNormalizeEmptyService(t *testing.T) {
	s := testSpan()
	s.Service = ""
	assert.Error(t, Normalize(s))
}

func TestNormalizeLongService(t *testing.T) {
	s := testSpan()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	assert.Error(t, Normalize(s))
}

func TestNormalizeNamePassThru(t *testing.T) {
	s := testSpan()
	before := s.Name
	Normalize(s)
	assert.Equal(t, before, s.Name)
}

func TestNormalizeEmptyName(t *testing.T) {
	s := testSpan()
	s.Name = ""
	assert.Error(t, Normalize(s))
}

func TestNormalizeLongName(t *testing.T) {
	s := testSpan()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.Error(t, Normalize(s))
}

func TestNormalizeName(t *testing.T) {
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	s := testSpan()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, Normalize(s))
		assert.Equal(t, expName, s.Name)
	}
}

func TestNormalizeNameFailure(t *testing.T) {
	invalidNames := []string{
		"",   // Empty.
		"/",  // No alphanumerics.
		"//", // Still no alphanumerics.
		strings.Repeat("x", MaxNameLen+1), // Too long.
	}
	s := testSpan()
	for _, v := range invalidNames {
		s.Name = v
		assert.Error(t, Normalize(s))
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	s := testSpan()
	before := s.Resource
	Normalize(s)
	assert.Equal(t, before, s.Resource)
}

func TestNormalizeEmptyResource(t *testing.T) {
	s := testSpan()
	s.Resource = ""
	assert.Error(t, Normalize(s))
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.TraceID
	Normalize(s)
	assert.Equal(t, before, s.TraceID)
}

func TestNormalizeNoTraceID(t *testing.T) {
	s := testSpan()
	s.TraceID = 0
	Normalize(s)
	assert.NotEqual(t, 0, s.TraceID)
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.SpanID
	Normalize(s)
	assert.Equal(t, before, s.SpanID)
}

func TestNormalizeNoSpanID(t *testing.T) {
	s := testSpan()
	s.SpanID = 0
	Normalize(s)
	assert.NotEqual(t, 0, s.SpanID)
}

func TestNormalizeStartPassThru(t *testing.T) {
	s := testSpan()
	before := s.Start
	Normalize(s)
	assert.Equal(t, before, s.Start)
}

func TestNormalizeStartTooSmall(t *testing.T) {
	s := testSpan()
	s.Start = 42
	assert.Error(t, Normalize(s))
}

func TestNormalizeStartTooLarge(t *testing.T) {
	s := testSpan()
	s.Start = time.Now().Add(15 * time.Minute).UnixNano()
	assert.Error(t, Normalize(s))
}

func TestNormalizeDurationPassThru(t *testing.T) {
	s := testSpan()
	before := s.Duration
	Normalize(s)
	assert.Equal(t, before, s.Duration)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	s := testSpan()
	s.Duration = 0
	assert.Error(t, Normalize(s))
}

func TestNormalizeNegativeDuration(t *testing.T) {
	s := testSpan()
	s.Duration = -50
	assert.Error(t, Normalize(s))
}

func TestNormalizeErrorPassThru(t *testing.T) {
	s := testSpan()
	before := s.Error
	Normalize(s)
	assert.Equal(t, before, s.Error)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	s := testSpan()
	before := s.Metrics
	Normalize(s)
	assert.Equal(t, before, s.Metrics)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	s := testSpan()
	before := s.Meta
	Normalize(s)
	assert.Equal(t, before, s.Meta)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.ParentID
	Normalize(s)
	assert.Equal(t, before, s.ParentID)
}

func TestNormalizeTypePassThru(t *testing.T) {
	s := testSpan()
	before := s.Type
	Normalize(s)
	assert.Equal(t, before, s.Type)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	s := testSpan()
	s.Type = strings.Repeat("sql", 1000)
	Normalize(s)
	assert.Error(t, Normalize(s))
}

func TestNormalizeServiceTag(t *testing.T) {
	s := testSpan()
	s.Service = "retargeting(api-Staging "
	Normalize(s)
	assert.Equal(t, "retargeting_api-staging", s.Service)
}

func TestNormalizeEnv(t *testing.T) {
	s := testSpan()
	s.Meta["env"] = "DEVELOPMENT"
	Normalize(s)
	assert.Equal(t, "development", s.Meta["env"])
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	s := testSpan()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	Normalize(s)
	assert.Equal(t, uint64(0), s.ParentID)
	assert.Equal(t, beforeTraceID, s.TraceID)
	assert.Equal(t, beforeSpanID, s.SpanID)
}

func TestNormalizeTraceEmpty(t *testing.T) {
	trace := pb.Trace{}

	err := NormalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceTraceIdMismatch(t *testing.T) {
	span1 := testSpan()
	span1.TraceID = 1

	span2 := testSpan()
	span2.TraceID = 2

	trace := pb.Trace{span1, span2}

	err := NormalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	span1 := testSpan()

	span2 := testSpan()
	span2.Name = "" // invalid

	trace := pb.Trace{span1, span2}

	err := NormalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	span1 := testSpan()
	span2 := testSpan()
	span2.SpanID = span1.SpanID

	trace := pb.Trace{span1, span2}

	err := NormalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTrace(t *testing.T) {
	span1 := testSpan()

	span2 := testSpan()
	span2.SpanID++

	trace := pb.Trace{span1, span2}

	err := NormalizeTrace(trace)
	assert.NoError(t, err)
}

func TestIsValidStatusCode(t *testing.T) {
	assert := assert.New(t)
	assert.True(isValidStatusCode("100"))
	assert.True(isValidStatusCode("599"))
	assert.False(isValidStatusCode("99"))
	assert.False(isValidStatusCode("600"))
	assert.False(isValidStatusCode("Invalid status code"))
}

func TestNormalizeInvalidUTF8(t *testing.T) {
	invalidUTF8 := "test\x99\x8f"

	t.Run("service", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Service = invalidUTF8

		err := Normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Service)
	})

	t.Run("resource", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Resource = invalidUTF8

		err := Normalize(span)

		assert.Nil(err)
		assert.Equal("test��", span.Resource)
	})

	t.Run("name", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Name = invalidUTF8

		err := Normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Name)
	})

	t.Run("type", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Type = invalidUTF8

		err := Normalize(span)

		assert.Nil(err)
		assert.Equal("test��", span.Type)
	})

	t.Run("meta", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Meta = map[string]string{
			invalidUTF8: "test1",
			"test2":     invalidUTF8,
		}

		err := Normalize(span)

		assert.Nil(err)
		assert.EqualValues(map[string]string{
			"test��": "test1",
			"test2":  "test��",
		}, span.Meta)
	})
}

func BenchmarkNormalization(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		Normalize(testSpan())
	}
}
