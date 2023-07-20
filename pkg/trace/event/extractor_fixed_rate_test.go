// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
)

func createTestSpans(serviceName string, operationName string) []*pb.Span {
	return createNTestSpans(1000, serviceName, operationName)
}

func createNTestSpans(n int, serviceName string, operationName string) []*pb.Span {
	spans := make([]*pb.Span, n)
	for i := range spans {
		spans[i] = &pb.Span{TraceID: rand.Uint64(), Service: serviceName, Name: operationName}
	}
	return spans
}

func TestFixedCases(t *testing.T) {
	assert := assert.New(t)
	e := NewFixedRateExtractor(map[string]map[string]float64{
		"sERvice1": {
			"OP1": 1,
			"op2": 0.5,
		},
	})

	span1 := &pb.Span{Service: "service1", Name: "op1"}
	span2 := &pb.Span{Service: "SerVice1", Name: "Op2"}

	rate, ok := e.Extract(span1, 0)
	assert.Equal(rate, 1.)
	assert.True(ok)

	rate, ok = e.Extract(span2, 0)
	assert.Equal(rate, 0.5)
	assert.True(ok)
}

func TestAnalyzedExtractor(t *testing.T) {
	config := make(map[string]map[string]float64)
	config["serviceA"] = make(map[string]float64)
	config["serviceA"]["opA"] = 0

	config["serviceB"] = make(map[string]float64)
	config["serviceB"]["opB"] = 0.5

	config["serviceC"] = make(map[string]float64)
	config["serviceC"]["opC"] = 1

	tests := []extractorTestCase{
		// Name: <priority>/(<no match reason>/<extraction rate>)
		{"none/noservice", createTestSpans("serviceZ", "opA"), 0, -1},
		{"none/noname", createTestSpans("serviceA", "opZ"), 0, -1},
		{"none/0", createTestSpans("serviceA", "opA"), 0, 0},
		{"none/0.5", createTestSpans("serviceB", "opB"), 0, 0.5},
		{"none/1", createTestSpans("serviceC", "opC"), 0, 1},
		{"1/noservice", createTestSpans("serviceZ", "opA"), 1, -1},
		{"1/noname", createTestSpans("serviceA", "opZ"), 1, -1},
		{"1/0", createTestSpans("serviceA", "opA"), 1, 0},
		{"1/0.5", createTestSpans("serviceB", "opB"), 1, 0.5},
		{"1/1", createTestSpans("serviceC", "opC"), 1, 1},
		{"2/noservice", createTestSpans("serviceZ", "opA"), 2, -1},
		{"2/noname", createTestSpans("serviceA", "opZ"), 2, -1},
		{"2/0", createTestSpans("serviceA", "opA"), 2, 0},
		{"2/0.5", createTestSpans("serviceB", "opB"), 2, 1},
		{"2/1", createTestSpans("serviceC", "opC"), 2, 1},
	}

	for _, test := range tests {
		testExtractor(t, NewFixedRateExtractor(config), test)
	}
}
