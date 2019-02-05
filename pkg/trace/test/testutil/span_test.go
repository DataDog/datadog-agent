package testutil

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopySpan(t *testing.T) {
	assert := assert.New(t)
	span := RandomSpan()
	cp := CopySpan(span)
	addr := func(val interface{}) uintptr { return reflect.ValueOf(val).Pointer() }

	assert.NotEqual(addr(cp), addr(span))
	assert.NotEqual(addr(cp.Meta), addr(span.Meta))
	assert.NotEqual(addr(cp.Metrics), addr(span.Metrics))
}
