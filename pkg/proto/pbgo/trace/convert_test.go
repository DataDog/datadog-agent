package trace

import (
	"testing"

	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
)

func TestConvertedSpan(t *testing.T) {
	v4Span := &Span{
		Service:  "my-service",
		Name:     "span-name",
		Resource: "GET /res",
		SpanID:   12345678,
		ParentID: 1111,
		Metrics: map[string]float64{
			"someNum": 1.0,
		},
		Meta: map[string]string{
			"someStr": "bar",
		},
		MetaStruct: map[string][]byte{
			"bts": []byte("bar"),
		},
		TraceID: 556677,
	}
	v4SpanBytes, err := v4Span.MarshalMsg(nil)
	assert.NoError(t, err)
	idxSpan := idx.NewInternalSpan(idx.NewStringTable(), &idx.Span{})
	convertedFields := idx.SpanConvertedFields{}
	o, err := idxSpan.UnmarshalMsgConverted(v4SpanBytes, &convertedFields)
	assert.NoError(t, err)
	assert.Empty(t, o)
	assert.Equal(t, "my-service", idxSpan.Service())
	assert.Equal(t, "span-name", idxSpan.Name())
	assert.Equal(t, "GET /res", idxSpan.Resource())
	assert.Equal(t, uint64(12345678), idxSpan.SpanID())
	assert.Equal(t, uint64(1111), idxSpan.ParentID())
	someNum, found := idxSpan.GetAttributeAsFloat64("someNum")
	assert.True(t, found)
	assert.Equal(t, float64(1.0), someNum)
	someStr, found := idxSpan.GetAttributeAsString("someStr")
	assert.True(t, found)
	assert.Equal(t, "bar", someStr)
	anyValue, found := idxSpan.GetAttribute("bts")
	assert.True(t, found)
	assert.Equal(t, &idx.AnyValue{
		Value: &idx.AnyValue_BytesValue{
			BytesValue: []byte("bar"),
		},
	}, anyValue)

	// Check for converted fields
	assert.Equal(t, uint64(556677), convertedFields.TraceIDLower)
}
