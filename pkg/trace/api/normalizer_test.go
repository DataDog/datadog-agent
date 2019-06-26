package api

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func newTestSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   rand.Uint64(),
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

// assertNormalizationIssue asserts there is exactly one NormalizationIssue in the provided TagStats matching the reason
func assertNormalizationIssue(t *testing.T, ts *info.TagStats, reason string) {
	expected := map[string]int64{reason: 1}

	found := make(map[string]int64)
	for r, c := range ts.AllNormalizationIssues() {
		if c > 0 {
			found[r] = c
		}
	}

	assert.EqualValues(t, expected, found, fmt.Sprintf("expected exactly one normalization issue: %s", reason))
}

// assertNormalizationIssue asserts there there are no NormalizationIssues in the provided TagStats
func assertNoNormalizationIssues(t *testing.T, ts *info.TagStats) {
	for reason, count := range ts.AllNormalizationIssues() {
		assert.EqualValues(t, 0, count, fmt.Sprintf("found unexpected normalization issue %s=%d, expecting no issues", reason, count))
	}
}

func TestNormalizeOK(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	assert.NoError(t, normalize(ts, s))
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeServicePassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Service
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Service)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyServiceNoLang(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Service = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Service, DefaultServiceName)
	assertNormalizationIssue(t, ts, "service_empty")
}

func TestNormalizeEmptyServiceWithLang(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Service = ""
	ts.Lang = "java"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Service, ts.Lang)
	assertNormalizationIssue(t, ts, "service_empty")
}

func TestNormalizeLongService(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Service = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Service, s.Service[:MaxServiceLen])
	assertNormalizationIssue(t, ts, "service_truncate")
}

func TestNormalizeNamePassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Name
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Name)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyName(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Name = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, DefaultSpanName)
	assertNormalizationIssue(t, ts, "span_name_empty")
}

func TestNormalizeLongName(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Name = strings.Repeat("CAMEMBERT", 100)
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, s.Name[:MaxNameLen])
	assertNormalizationIssue(t, ts, "span_name_truncate")
}

func TestNormalizeNameNoAlphanumeric(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Name = "/"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Name, DefaultSpanName)
	assertNormalizationIssue(t, ts, "span_name_invalid")
}

func TestNormalizeNameForMetrics(t *testing.T) {
	expNames := map[string]string{
		"pylons.controller": "pylons.controller",
		"trace-api.request": "trace_api.request",
	}

	ts, s := &info.TagStats{}, newTestSpan()
	for name, expName := range expNames {
		s.Name = name
		assert.NoError(t, normalize(ts, s))
		assert.Equal(t, expName, s.Name)
		assertNoNormalizationIssues(t, ts)
	}
}

func TestNormalizeResourcePassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Resource
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Resource)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyResource(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Resource = ""
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, s.Resource, s.Name)
	assertNormalizationIssue(t, ts, "resource_empty")
}

func TestNormalizeTraceIDPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.TraceID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.TraceID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeNoTraceID(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.TraceID = 0
	assert.Error(t, normalize(ts, s))
	assertNormalizationIssue(t, ts, "trace_id_zero")
}

func TestNormalizeSpanIDPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.SpanID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.SpanID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeNoSpanID(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.SpanID = 0
	assert.Error(t, normalize(ts, s))
	assertNormalizationIssue(t, ts, "span_id_zero")
}

func TestNormalizeStartPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Start
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Start)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeStartTooSmall(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Start = 42
	assert.NoError(t, normalize(ts, s))
	assertNormalizationIssue(t, ts, "invalid_start_date")

}

func TestNormalizeDurationPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Duration
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Duration)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEmptyDuration(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Duration = 0
	assert.NoError(t, normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeNegativeDuration(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Duration = -50
	assert.NoError(t, normalize(ts, s))
	assert.EqualValues(t, s.Duration, 0)
	assertNormalizationIssue(t, ts, "invalid_duration")
}

func TestNormalizeErrorPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Error
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Error)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeMetricsPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Metrics
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Metrics)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeMetaPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Meta
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Meta)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeParentIDPassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.ParentID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.ParentID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeTypePassThru(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	before := s.Type
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, before, s.Type)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeTypeTooLong(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Type = strings.Repeat("sql", 1000)
	assert.NoError(t, normalize(ts, s))
	assertNormalizationIssue(t, ts, "type_truncate")
}

func TestNormalizeServiceTag(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Service = "retargeting(api-Staging "
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, "retargeting_api-staging", s.Service)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeEnv(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.Meta["env"] = "DEVELOPMENT"
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, "development", s.Meta["env"])
	assertNoNormalizationIssues(t, ts)
}

func TestSpecialZipkinRootSpan(t *testing.T) {
	ts, s := &info.TagStats{}, newTestSpan()
	s.ParentID = 42
	s.TraceID = 42
	s.SpanID = 42
	beforeTraceID := s.TraceID
	beforeSpanID := s.SpanID
	assert.NoError(t, normalize(ts, s))
	assert.Equal(t, uint64(0), s.ParentID)
	assert.Equal(t, beforeTraceID, s.TraceID)
	assert.Equal(t, beforeSpanID, s.SpanID)
	assertNoNormalizationIssues(t, ts)
}

func TestNormalizeTraceEmpty(t *testing.T) {
	ts, trace := &info.TagStats{}, pb.Trace{}
	err := normalizeTrace(ts, trace)
	assert.Error(t, err)
	assertNormalizationIssue(t, ts, "empty_trace")
}

func TestNormalizeTraceTraceIdMismatch(t *testing.T) {
	ts, span1, span2 := &info.TagStats{}, newTestSpan(), newTestSpan()
	span1.TraceID = 1
	span2.TraceID = 2
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.Error(t, err)
	assertNormalizationIssue(t, ts, "foreign_span")
}

func TestNormalizeTraceInvalidSpan(t *testing.T) {
	ts, span1, span2 := &info.TagStats{}, newTestSpan(), newTestSpan()
	span2.Name = "" // invalid
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assertNormalizationIssue(t, ts, "span_name_empty")
}

func TestNormalizeTraceDuplicateSpanID(t *testing.T) {
	ts, span1, span2 := &info.TagStats{}, newTestSpan(), newTestSpan()
	span2.SpanID = span1.SpanID
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
	assert.NoError(t, err)
	assertNormalizationIssue(t, ts, "duplicate_span_id")
}

func TestNormalizeTrace(t *testing.T) {
	ts, span1, span2 := &info.TagStats{}, newTestSpan(), newTestSpan()
	span2.SpanID++
	trace := pb.Trace{span1, span2}
	err := normalizeTrace(ts, trace)
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

		ts, span := &info.TagStats{}, newTestSpan()
		span.Service = invalidUTF8

		err := normalize(ts, span)

		assert.Nil(err)
		assert.Equal("test", span.Service)
	})

	t.Run("resource", func(t *testing.T) {
		assert := assert.New(t)

		ts, span := &info.TagStats{}, newTestSpan()
		span.Resource = invalidUTF8

		err := normalize(ts, span)

		assert.Nil(err)
		assert.Equal("test��", span.Resource)
	})

	t.Run("name", func(t *testing.T) {
		assert := assert.New(t)

		ts, span := &info.TagStats{}, newTestSpan()
		span.Name = invalidUTF8

		err := normalize(ts, span)

		assert.Nil(err)
		assert.Equal("test", span.Name)
	})

	t.Run("type", func(t *testing.T) {
		assert := assert.New(t)

		ts, span := &info.TagStats{}, newTestSpan()
		span.Type = invalidUTF8

		err := normalize(ts, span)

		assert.Nil(err)
		assert.Equal("test��", span.Type)
	})

	t.Run("meta", func(t *testing.T) {
		assert := assert.New(t)

		ts, span := &info.TagStats{}, newTestSpan()
		span.Meta = map[string]string{
			invalidUTF8: "test1",
			"test2":     invalidUTF8,
		}

		err := normalize(ts, span)

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
		ts, span := &info.TagStats{}, newTestSpan()
		normalize(ts, span)
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
