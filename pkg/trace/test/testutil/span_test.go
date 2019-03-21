package testutil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func memaddr(val interface{}) uintptr { return reflect.ValueOf(val).Pointer() }

func TestCopySpan(t *testing.T) {
	assert := assert.New(t)
	span := RandomSpan()
	cp := CopySpan(span)

	assert.Equal(cp.Service, span.Service)
	assert.Equal(cp.Name, span.Name)
	assert.Equal(cp.Resource, span.Resource)
	assert.Equal(cp.TraceID, span.TraceID)
	assert.Equal(cp.SpanID, span.SpanID)
	assert.Equal(cp.ParentID, span.ParentID)
	assert.Equal(cp.Start, span.Start)
	assert.Equal(cp.Duration, span.Duration)
	assert.Equal(cp.Error, span.Error)
	assert.Equal(cp.Type, span.Type)

	assert.NotEqual(memaddr(cp), memaddr(span))
	assert.NotEqual(memaddr(cp.Meta), memaddr(span.Meta))
	assert.NotEqual(memaddr(cp.Metrics), memaddr(span.Metrics))
}
