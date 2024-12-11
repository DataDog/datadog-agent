// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sampler

import (
	"hash/fnv"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/rand"
)

func testComputeSignature(trace pb.Trace, env string) Signature {
	root := traceutil.GetRoot(trace)
	return computeSignatureWithRootAndEnv(trace, root, env)
}

func TestSignatureSimilar(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 102, SpanID: 1021, Service: "x1", Name: "y1", Resource: "z1", Duration: 992312},
		&pb.Span{TraceID: 102, SpanID: 1022, ParentID: 1021, Service: "x1", Name: "y1", Resource: "z1", Duration: 34347},
		&pb.Span{TraceID: 102, SpanID: 1023, ParentID: 1022, Service: "x2", Name: "y2", Resource: "z2", Duration: 349944},
	}

	assert.Equal(testComputeSignature(t1, ""), testComputeSignature(t2, ""))
}

func TestSignatureDifferentError(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 110, SpanID: 1101, Service: "x1", Name: "y1", Resource: "z1", Duration: 992312},
		&pb.Span{TraceID: 110, SpanID: 1102, ParentID: 1101, Service: "x1", Name: "y1", Resource: "z1", Error: 1, Duration: 34347},
		&pb.Span{TraceID: 110, SpanID: 1103, ParentID: 1101, Service: "x2", Name: "y2", Resource: "z2", Duration: 349944},
	}

	assert.NotEqual(testComputeSignature(t1, ""), testComputeSignature(t2, ""))
}

func TestSignatureDifferentRoot(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 103, SpanID: 1031, Service: "x1", Name: "y1", Resource: "z2", Duration: 19207},
		&pb.Span{TraceID: 103, SpanID: 1032, ParentID: 1031, Service: "x1", Name: "y1", Resource: "z1", Duration: 234923874},
		&pb.Span{TraceID: 103, SpanID: 1033, ParentID: 1032, Service: "x1", Name: "y1", Resource: "z1", Duration: 152342344},
	}

	assert.NotEqual(testComputeSignature(t1, ""), testComputeSignature(t2, ""))
}

func TestSignatureDifference(t *testing.T) {
	type testCase struct {
		name string
		meta map[string]string
	}
	testCases := []testCase{
		{"status-code", map[string]string{KeyHTTPStatusCode: "200"}},
		{"error-type", map[string]string{KeyErrorType: "error: nil"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert := assert.New(t)
			t1 := pb.Trace{
				&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
			}
			t2 := pb.Trace{
				&pb.Span{TraceID: 103, SpanID: 1031, Service: "x1", Name: "y1", Resource: "z1", Duration: 19207, Meta: tc.meta},
			}
			assert.NotEqual(testComputeSignature(t1, ""), testComputeSignature(t2, ""))
		})
	}
}

func testComputeServiceSignature(trace pb.Trace, env string) Signature {
	root := traceutil.GetRoot(trace)
	return ServiceSignature{root.Service, env}.Hash()
}

func TestServiceSignatureSimilar(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 102, SpanID: 1021, Service: "x1", Name: "y2", Resource: "z2", Duration: 992312},
		&pb.Span{TraceID: 102, SpanID: 1022, ParentID: 1021, Service: "x1", Name: "y1", Resource: "z1", Error: 1, Duration: 34347},
		&pb.Span{TraceID: 102, SpanID: 1023, ParentID: 1022, Service: "x2", Name: "y2", Resource: "z2", Duration: 349944},
	}
	assert.Equal(testComputeServiceSignature(t1, ""), testComputeServiceSignature(t2, ""))
}

func TestServiceSignatureDifferentService(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 103, SpanID: 1031, Service: "x2", Name: "y1", Resource: "z1", Duration: 19207},
		&pb.Span{TraceID: 103, SpanID: 1032, ParentID: 1031, Service: "x1", Name: "y1", Resource: "z1", Duration: 234923874},
		&pb.Span{TraceID: 103, SpanID: 1033, ParentID: 1032, Service: "x1", Name: "y1", Resource: "z1", Duration: 152342344},
	}

	assert.NotEqual(testComputeServiceSignature(t1, ""), testComputeServiceSignature(t2, ""))
}

func TestServiceSignatureDifferentEnv(t *testing.T) {
	assert := assert.New(t)

	t1 := pb.Trace{
		&pb.Span{TraceID: 101, SpanID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 26965},
		&pb.Span{TraceID: 101, SpanID: 1012, ParentID: 1011, Service: "x1", Name: "y1", Resource: "z1", Duration: 197884},
		&pb.Span{TraceID: 101, SpanID: 1013, ParentID: 1012, Service: "x1", Name: "y1", Resource: "z1", Duration: 12304982304},
		&pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993},
	}
	t2 := pb.Trace{
		&pb.Span{TraceID: 110, SpanID: 1101, Service: "x1", Name: "y1", Resource: "z1", Duration: 992312},
		&pb.Span{TraceID: 110, SpanID: 1102, ParentID: 1101, Service: "x1", Name: "y1", Resource: "z1", Duration: 34347},
		&pb.Span{TraceID: 110, SpanID: 1103, ParentID: 1101, Service: "x2", Name: "y2", Resource: "z2", Duration: 349944},
	}

	assert.NotEqual(testComputeServiceSignature(t1, "test"), testComputeServiceSignature(t2, "prod"))
}

func TestSum32a(t *testing.T) {
	assert := assert.New(t)
	testList := []string{"this", "is", "just", "a", "sanity", "check", "Съешь же ещё этих мягких французских булок да выпей чаю"}
	for _, s := range testList {
		h := fnv.New32a()
		h.Write([]byte(s))
		expected := h.Sum32()

		h2 := new32a()
		h2.Write([]byte(s))
		actual := h2.Sum32()

		assert.Equal(expected, actual)
	}
}

func BenchmarkServiceSignature_Hash(b *testing.B) {
	s1 := rand.String(10)
	s2 := rand.String(10)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ServiceSignature{s1, s2}.Hash()
	}
}

func BenchmarkComputeSpanHash(b *testing.B) {
	span := &pb.Span{TraceID: 101, SpanID: 1014, ParentID: 1013, Service: "x2", Name: "y2", Resource: "z2", Duration: 34384993}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		computeSpanHash(span, "prod", true)
	}
}
