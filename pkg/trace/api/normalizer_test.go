package api

import (
	"bytes"
	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func testObjects() (*info.TagStats, *pb.Span) {
	ts := &info.TagStats{info.Tags{}, info.Stats{}}
	s := &pb.Span{
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
	return ts, s
}

func assertNormalizationIssue(t *testing.T, ts *info.TagStats, reason string) {
	normalizationIssues := append(ts.DroppedTraceNormalizationIssues(), ts.MalformedTraceNormalizationIssues()...)
	for _, issue := range normalizationIssues {
		if issue.Reason == reason {
			assert.Equal(t, issue.Count, 1)
		} else {
			assert.Equal(t, issue.Count, 0)
		}
	}
}

func assertNoNormalizationIssues(t *testing.T, ts *info.TagStats) {
	allReasonCounts := append(ts.DroppedTraceNormalizationIssues(), ts.MalformedTraceNormalizationIssues()...)
	for _, reasonCount := range allReasonCounts {
		assert.Equal(t, reasonCount.Count, 0)
	}
}

func TestNormalizeOK(t *testing.T) {
	ts, s := testObjects()
	assert.NoError(t, normalize(ts, s, s))
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeServicePassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Service
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.Service)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyServiceNoLang(t *testing.T) {
	ts, s := testObjects()
	s.Service = ""
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Service, DefaultServiceName)
	assertNormalizationIssue(t, ts, "service_empty")
}

func TestNormalizeEmptyServiceWithLang(t *testing.T) {
	ts, s := testObjects()
	s.Service = ""
	ts.Lang = "java"
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Service, ts.Lang)
	assertNormalizationIssue(t, ts, "service_empty")
}

func TestNormalizeLongService(t *testing.T) {
	ts, s := testObjects()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Service, s.Service[:MaxServiceLen])
	assertNormalizationIssue(t, ts, "service_truncate")
}

func TestNormalizeNamePassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Name
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.Name)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyName(t *testing.T) {
	ts, s := testObjects()
	s.Name = ""
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Name, DefaultSpanName)
	assertNormalizationIssue(t, ts, "span_name_empty")
}

func TestNormalizeLongName(t *testing.T) {
	ts, s := testObjects()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Name, s.Name[:MaxNameLen])
	assertNormalizationIssue(t, ts, "span_name_truncate")
}

func TestNormalizeName(t *testing.T) {
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	ts, s := testObjects()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, normalize(ts, s, s))
		assert.Equal(t, expName, s.Name)
		assertNoNormalizationIssues(t, ts)
	}
}

func TestNormalizeNameFailure(t *testing.T) {
	invalidNames := []string{
		"",                                // Empty.
		"/",                               // No alphanumerics.
		"//",                              // Still no alphanumerics.
		strings.Repeat("x", MaxNameLen+1), // Too long.
	}
	for _, v := range invalidNames {
		ts, s := testObjects()
		s.Name = v
		assert.NoError(t, normalize(ts, s, s))
		assert.Equal(t, s.Name, DefaultSpanName)
		assertNormalizationIssue(t, ts, "span_name_invalid")
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Resource
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.Resource)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyResource(t *testing.T) {
	ts, s := testObjects()
	s.Resource = ""
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, s.Resource, s.Name)
	assertNormalizationIssue(t, ts, "resource_name_empty")
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.TraceID
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.TraceID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeNoTraceID(t *testing.T) {
	ts, s := testObjects()
	s.TraceID = 0
	assert.Error(t, normalize(ts, s, s))
	assertNormalizationIssue(t, ts, "trace_id_zero")
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.SpanID
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.SpanID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeNoSpanID(t *testing.T) {
	ts, s := testObjects()
	s.SpanID = 0
	assert.Error(t, normalize(ts, s, s))
	assertNormalizationIssue(t, ts, "span_id_zero")
}

func TestNormalizeStartPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Start
	assert.NoError(t, normalize(ts, s, s))
	assert.Equal(t, before, s.Start)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeStartTooSmall(t *testing.T) {
	ts, s := testObjects()
	s.Start = 42
	assert.NoError(t, normalize(ts, s, s))
	assertNormalizationIssue(t, ts, "invalid_start_date")

}

func TestNormalizeStartTooLarge(t *testing.T) {
	ts, s := testObjects()
	s.Start = time.Now().Add(15 * time.Minute).UnixNano()
	assert.Error(t, normalize(ts, s, s))
}

func TestNormalizeDurationPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Duration
	normalize(ts, s, s)
	assert.Equal(t, before, s.Duration)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	ts, s := testObjects()
	s.Duration = 0
	assert.Error(t, normalize(ts, s, s))
}

func TestNormalizeNegativeDuration(t *testing.T) {
	ts, s := testObjects()
	s.Duration = -50
	assert.Error(t, normalize(ts, s, s))
}

func TestNormalizeErrorPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Error
	normalize(ts, s, s)
	assert.Equal(t, before, s.Error)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Metrics
	normalize(ts, s, s)
	assert.Equal(t, before, s.Metrics)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Meta
	normalize(ts, s, s)
	assert.Equal(t, before, s.Meta)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.ParentID
	normalize(ts, s, s)
	assert.Equal(t, before, s.ParentID)
}

func TestNormalizeTypePassThru(t *testing.T) {
	ts, s := testObjects()
	before := s.Type
	normalize(ts, s, s)
	assert.Equal(t, before, s.Type)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	ts, s := testObjects()
	s.Type = strings.Repeat("sql", 1000)
	normalize(ts, s, s)
	assert.Error(t, normalize(ts, s, s))
}

func TestNormalizeServiceTag(t *testing.T) {
	ts, s := testObjects()
	s.Service = "retargeting(api-Staging "
	normalize(ts, s, s)
	assert.Equal(t, "retargeting_api-staging", s.Service)
}

func TestNormalizeEnv(t *testing.T) {
	ts, s := testObjects()
	s.Meta["env"] = "DEVELOPMENT"
	normalize(ts, s, s)
	assert.Equal(t, "development", s.Meta["env"])
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	ts, s := testObjects()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	normalize(ts, s, s)
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
	span1 := testObjects()
	span1.TraceID = 1

	span2 := testObjects()
	span2.TraceID = 2

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	span1 := testObjects()

	span2 := testObjects()
	span2.Name = "" // invalid

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	span1 := testObjects()
	span2 := testObjects()
	span2.SpanID = span1.SpanID

	trace := pb.Trace{span1, span2}

	err := normalizeTrace(trace)
	assert.Error(t, err)
}

func TestNormalizeTrace(t *testing.T) {
	span1 := testObjects()

	span2 := testObjects()
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

		span := testObjects()
		span.Service = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Service)
	})

	t.Run("resource", func(t *testing.T) {
		assert := assert.New(t)

		span := testObjects()
		span.Resource = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test��", span.Resource)
	})

	t.Run("name", func(t *testing.T) {
		assert := assert.New(t)

		span := testObjects()
		span.Name = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test", span.Name)
	})

	t.Run("type", func(t *testing.T) {
		assert := assert.New(t)

		span := testObjects()
		span.Type = invalidUTF8

		err := normalize(span)

		assert.Nil(err)
		assert.Equal("test��", span.Type)
	})

	t.Run("meta", func(t *testing.T) {
		assert := assert.New(t)

		span := testObjects()
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
		normalize(testObjects())
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
		{in: "test\x99\x8faaa", out: "test_aaa"},
		{in: "test\x99\x8f", out: "test"},
		{in: strings.Repeat("a", 888), out: strings.Repeat("a", 200)},
		{
			in: func() string {
				b := bytes.NewBufferString("a")
				for i := 0; i < 799; i++ {
					_, err := b.WriteRune('🐶')
					assert.NoError(t, err)
				}
				_, err := b.WriteRune('b')
				assert.NoError(t, err)
				return b.String()
			}(),
			out: "a", // 'b' should have been truncated
		},
		{"a" + string(unicode.ReplacementChar), "a"},
		{"a" + string(unicode.ReplacementChar) + string(unicode.ReplacementChar), "a"},
		{"a" + string(unicode.ReplacementChar) + string(unicode.ReplacementChar) + "b", "a_b"},
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
