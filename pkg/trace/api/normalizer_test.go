package api

import (
	"strings"
	"testing"
	"time"

	"github.com/StackVista/stackstate-agent/pkg/trace/pb"
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
	assert.NoError(t, normalize(s))
}

func TestNormalizeServicePassThru(t *testing.T) {
	s := testSpan()
	before := s.Service
	normalize(s)
	assert.Equal(t, before, s.Service)
}

func TestNormalizeEmptyService(t *testing.T) {
	s := testSpan()
	s.Service = ""
	assert.Error(t, normalize(s))
}

func TestNormalizeLongService(t *testing.T) {
	s := testSpan()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	assert.Error(t, normalize(s))
}

func TestNormalizeNamePassThru(t *testing.T) {
	s := testSpan()
	before := s.Name
	normalize(s)
	assert.Equal(t, before, s.Name)
}

func TestNormalizeEmptyName(t *testing.T) {
	s := testSpan()
	s.Name = ""
	assert.Error(t, normalize(s))
}

func TestNormalizeLongName(t *testing.T) {
	s := testSpan()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.Error(t, normalize(s))
}

func TestNormalizeName(t *testing.T) {
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	s := testSpan()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, normalize(s))
		assert.Equal(t, expName, s.Name)
	}
}

func TestNormalizeNameFailure(t *testing.T) {
	invalidNames := []string{
		"",                                // Empty.
		"/",                               // No alphanumerics.
		"//",                              // Still no alphanumerics.
		strings.Repeat("x", MaxNameLen+1), // Too long.
	}
	s := testSpan()
	for _, v := range invalidNames {
		s.Name = v
		assert.Error(t, normalize(s))
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	s := testSpan()
	before := s.Resource
	normalize(s)
	assert.Equal(t, before, s.Resource)
}

func TestNormalizeEmptyResource(t *testing.T) {
	s := testSpan()
	s.Resource = ""
	assert.Error(t, normalize(s))
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.TraceID
	normalize(s)
	assert.Equal(t, before, s.TraceID)
}

func TestNormalizeNoTraceID(t *testing.T) {
	s := testSpan()
	s.TraceID = 0
	normalize(s)
	assert.NotEqual(t, 0, s.TraceID)
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.SpanID
	normalize(s)
	assert.Equal(t, before, s.SpanID)
}

func TestNormalizeNoSpanID(t *testing.T) {
	s := testSpan()
	s.SpanID = 0
	normalize(s)
	assert.NotEqual(t, 0, s.SpanID)
}

func TestNormalizeStartPassThru(t *testing.T) {
	s := testSpan()
	before := s.Start
	normalize(s)
	assert.Equal(t, before, s.Start)
}

func TestNormalizeStartTooSmall(t *testing.T) {
	s := testSpan()
	s.Start = 42
	assert.Error(t, normalize(s))
}

func TestNormalizeStartTooLarge(t *testing.T) {
	s := testSpan()
	s.Start = time.Now().Add(15 * time.Minute).UnixNano()
	assert.Error(t, normalize(s))
}

func TestNormalizeDurationPassThru(t *testing.T) {
	s := testSpan()
	before := s.Duration
	normalize(s)
	assert.Equal(t, before, s.Duration)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	s := testSpan()
	s.Duration = 0
	assert.Error(t, normalize(s))
}

func TestNormalizeNegativeDuration(t *testing.T) {
	s := testSpan()
	s.Duration = -50
	assert.Error(t, normalize(s))
}

func TestNormalizeErrorPassThru(t *testing.T) {
	s := testSpan()
	before := s.Error
	normalize(s)
	assert.Equal(t, before, s.Error)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	s := testSpan()
	before := s.Metrics
	normalize(s)
	assert.Equal(t, before, s.Metrics)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	s := testSpan()
	before := s.Meta
	normalize(s)
	assert.Equal(t, before, s.Meta)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	s := testSpan()
	before := s.ParentID
	normalize(s)
	assert.Equal(t, before, s.ParentID)
}

func TestNormalizeTypePassThru(t *testing.T) {
	s := testSpan()
	before := s.Type
	normalize(s)
	assert.Equal(t, before, s.Type)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	s := testSpan()
	s.Type = strings.Repeat("sql", 1000)
	normalize(s)
	assert.Error(t, normalize(s))
}

func TestNormalizeServiceTag(t *testing.T) {
	s := testSpan()
	s.Service = "retargeting(api-Staging "
	normalize(s)
	assert.Equal(t, "retargeting_api-staging", s.Service)
}

func TestNormalizeEnv(t *testing.T) {
	s := testSpan()
	s.Meta["env"] = "DEVELOPMENT"
	normalize(s)
	assert.Equal(t, "development", s.Meta["env"])
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	s := testSpan()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	normalize(s)
	assert.Equal(t, uint64(0), s.ParentID)
	assert.Equal(t, beforeTraceID, s.TraceID)
	assert.Equal(t, beforeSpanID, s.SpanID)
}

func TestNormalizeTraceEmpty(t *testing.T) {
	trace := pb.Trace{}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceTraceIdMismatch(t *testing.T) {
	span1 := testSpan()
	span1.TraceID = 1

	span2 := testSpan()
	span2.TraceID = 2

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	span1 := testSpan()

	span2 := testSpan()
	span2.Name = "" // invalid

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	span1 := testSpan()
	span2 := testSpan()
	span2.SpanID = span1.SpanID

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTrace(t *testing.T) {
	span1 := testSpan()

	span2 := testSpan()
	span2.SpanID++

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
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

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Service)
	})

	t.Run("resource", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Resource = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test��", span.Resource)
	})

	t.Run("name", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Name = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Name)
	})

	t.Run("type", func(t *testing.T) {
		assert := assert.New(t)

		span := testSpan()
		span.Type = invalidUTF8

		err := normalize(span)

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

		err := normalize(span)

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
		normalize(testSpan())
	}
}

func TestNormalizeTag(t *testing.T) {
	for _, tt := range []struct{ in, out string }{
		{in: "#test_starting_hash", out: "test_starting_hash"},
		{in: "TestCAPSandSuch", out: "testcapsandsuch"},
		{in: "Test Conversion Of Weird !@#$%^&**() Characters", out: "test_conversion_of_weird_characters"},
		{in: "$#weird_starting", out: "weird_starting"},
		{in: "allowed:c0l0ns", out: "allowed:c0l0ns"},
		{in: "1love", out: "love"},
		{in: "ünicöde", out: "ünicöde"},
		{in: "ünicöde:metäl", out: "ünicöde:metäl"},
		{in: "Data🐨dog🐶 繋がっ⛰てて", out: "data_dog_繋がっ_てて"},
		{in: " spaces   ", out: "spaces"},
		{in: " #hashtag!@#spaces #__<>#  ", out: "hashtag_spaces"},
		{in: ":testing", out: ":testing"},
		{in: "_foo", out: "foo"},
		{in: ":::test", out: ":::test"},
		{in: "contiguous_____underscores", out: "contiguous_underscores"},
		{in: "foo_", out: "foo"},
		{in: "\u017Fodd_\u017Fcase\u017F", out: "\u017Fodd_\u017Fcase\u017F"}, // edge-case
		{in: "", out: ""},
		{in: " ", out: ""},
		{in: "ok", out: "ok"},
		{in: "™Ö™Ö™™Ö™", out: "ö_ö_ö"},
		{in: "AlsO:ök", out: "also:ök"},
		{in: ":still_ok", out: ":still_ok"},
		{in: "___trim", out: "trim"},
		{in: "12.:trim@", out: ":trim"},
		{in: "12.:trim@@", out: ":trim"},
		{in: "fun:ky__tag/1", out: "fun:ky_tag/1"},
		{in: "fun:ky@tag/2", out: "fun:ky_tag/2"},
		{in: "fun:ky@@@tag/3", out: "fun:ky_tag/3"},
		{in: "tag:1/2.3", out: "tag:1/2.3"},
		{in: "---fun:k####y_ta@#g/1_@@#", out: "fun:k_y_ta_g/1"},
		{in: "AlsO:œ#@ö))œk", out: "also:œ_ö_œk"},
	} {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, tt.out, normalizeTag(tt.in), tt.in)
		})
	}
}

func benchNormalizeTag(tag string) func(b *testing.B) {
	return func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			normalizeTag(tag)
		}
	}
}

func BenchmarkNormalizeTag(b *testing.B) {
	b.Run("ok", benchNormalizeTag("good_tag"))
	b.Run("trim", benchNormalizeTag("___trim_left"))
	b.Run("trim-both", benchNormalizeTag("___trim_right@@#!"))
	b.Run("plenty", benchNormalizeTag("fun:ky_ta@#g/1"))
	b.Run("more", benchNormalizeTag("fun:k####y_ta@#g/1_@@#"))
}
