package api

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"

	"github.com/tinylib/msgp/msgp"
	"github.com/stretchr/testify/assert"
)

func TestEncodeDecodeTraces(t *testing.T) {
	assert := assert.New(t)
	v := pb.Traces{
		[]*pb.Span{
			&pb.Span{
				Service: "service",
				Name: "name",
				Resource: "resource",
				TraceID: 1234,
				SpanID: 5678,
				ParentID: 9012,
				Start: 3456,
				Duration: 7890,
				Error: 2468,
				Meta: map[string]string{ "abcd": "efgh", "ijkl": "mnop" },
				Metrics: map[string]float64 { "abcd": 1234, "efgh": 5678 },
				Type: "type",				
			},
			&pb.Span{
				Service: "service2",
				Name: "name2",
				Resource: "resource2",
				TraceID: 12342,
				SpanID: 56782,
				ParentID: 90122,
				Start: 34562,
				Duration: 78902,
				Error: 24682,
				Meta: map[string]string{ "abcd2": "efgh2", "ijkl2": "mnop2" },
				Metrics: map[string]float64 { "abcd2": 12342, "efgh2": 56782 },
				Type: "type2",				
			},
		},
		[]*pb.Span{
			&pb.Span{
				Service: "service3",
				Name: "name3",
				Resource: "resource3",
				TraceID: 12343,
				SpanID: 56783,
				ParentID: 90123,
				Start: 34563,
				Duration: 78903,
				Error: 24683,
				Meta: map[string]string{ "abcd3": "efgh3", "ijkl3": "mnop3" },
				Metrics: map[string]float64 { "abcd3": 12343, "efgh3": 56783 },
				Type: "type3",				
			},
			&pb.Span{
				Service: "service4",
				Name: "name4",
				Resource: "resource4",
				TraceID: 12344,
				SpanID: 56784,
				ParentID: 90124,
				Start: 34564,
				Duration: 78904,
				Error: 24684,
				Meta: map[string]string{ "abcd4": "efgh4", "ijkl4": "mnop4" },
				Metrics: map[string]float64 { "abcd4": 12344, "efgh4": 56784 },
				Type: "type4",				
			},
		},
	}
	var buf bytes.Buffer
	msgp.Encode(&buf, &v)

	m := v.Msgsize()
	if buf.Len() > m {
		t.Logf("WARNING: Msgsize() for %v is inaccurate", v)
	}
	
	bs := buf.Bytes()

	vn := pb.Traces{}
	err := directDecodeTraces(bs, &vn)
	if err != nil {
		t.Error(err)
	}
	//v[0][0].Meta["abcd"] = "aoeuao.puaouea"
	//Service = "spervice"
	assert.Equal(v, vn)
}