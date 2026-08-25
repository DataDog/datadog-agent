// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package otelprocessctx

import (
	"encoding/binary"
	"os"
	"testing"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/probe/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// selfMemory reads this process's own address space, the way the resolver reads a
// target's.
func selfMemory(t *testing.T) procfs.Mem {
	t.Helper()

	mem, err := procfs.OpenMem(uint32(os.Getpid()))
	require.NoError(t, err)
	t.Cleanup(func() { mem.Close() })

	return mem
}

// publishContext publishes payload as the process context of this process, and returns
// the address of its header. The payload gets a mapping of its own, so that it is only
// reachable through the pointer the header carries, the way a publisher's heap buffer
// is.
func publishContext(t *testing.T, payload []byte) uint64 {
	t.Helper()

	header := anonymousMapping(t)
	payloadMapping := anonymousMapping(t)
	copy(payloadMapping, payload)

	copy(header[0:8], signature[:])
	binary.NativeEndian.PutUint32(header[8:12], headerVersion)
	binary.NativeEndian.PutUint32(header[12:16], uint32(len(payload)))
	binary.NativeEndian.PutUint64(header[24:32], addressOf(payloadMapping))
	// Published last, as the protocol requires.
	binary.NativeEndian.PutUint64(header[16:24], 1)

	return addressOf(header)
}

func anonymousMapping(t *testing.T) []byte {
	t.Helper()

	mapping, err := unix.Mmap(-1, 0, os.Getpagesize(), unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Munmap(mapping) })
	return mapping
}

func addressOf(mapping []byte) uint64 {
	return uint64(uintptr(unsafe.Pointer(&mapping[0])))
}

// Protobuf builders for the extra attributes of a process context.

func pbBytes(field int, value []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(field)<<3|wireBytes)
	out = binary.AppendUvarint(out, uint64(len(value)))
	return append(out, value...)
}

func pbVarint(field int, value uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(field)<<3|wireVarint), value)
}

func pbKeyValue(key string, value []byte) []byte {
	kv := pbBytes(fieldKeyValueKey, []byte(key))
	return append(kv, pbBytes(fieldKeyValueValue, value)...)
}

// pbAttribute builds one extra attribute, pbResource a resource holding one attribute.
func pbAttribute(key string, value []byte) []byte {
	return pbBytes(fieldProcessCtxExtraAttributes, pbKeyValue(key, value))
}

func pbResource(key string, value []byte) []byte {
	return pbBytes(fieldProcessCtxResource, pbBytes(fieldResourceAttributes, pbKeyValue(key, value)))
}

func pbString(s string) []byte { return pbBytes(fieldAnyValueString, []byte(s)) }

func pbInt(i int64) []byte { return pbVarint(fieldAnyValueInt, uint64(i)) }

func pbStringArray(entries ...string) []byte {
	var array []byte
	for _, entry := range entries {
		array = append(array, pbBytes(fieldArrayValueValues, pbString(entry))...)
	}
	return pbBytes(fieldAnyValueArray, array)
}

// TestReadPublishedContext reads a published context back, with one attribute of each
// kind this reader decodes.
func TestReadPublishedContext(t *testing.T) {
	var payload []byte
	payload = append(payload, pbResource("a.resource.attribute", pbString("resource"))...)
	payload = append(payload, pbAttribute("a.string", pbString("published"))...)
	payload = append(payload, pbAttribute("an.int", pbInt(42))...)
	payload = append(payload, pbAttribute("a.string.slice", pbStringArray("first", "second"))...)

	ctx, err := Read(selfMemory(t), publishContext(t, payload))
	require.NoError(t, err)

	resource, ok := ctx.Resource.String("a.resource.attribute")
	require.True(t, ok)
	assert.Equal(t, "resource", resource)

	attrs := ctx.Attributes
	str, ok := attrs.String("a.string")
	require.True(t, ok)
	assert.Equal(t, "published", str)

	number, ok := attrs.Int("an.int")
	require.True(t, ok)
	assert.Equal(t, int64(42), number)

	slice, ok := attrs.StringSlice("a.string.slice")
	require.True(t, ok)
	assert.Equal(t, []string{"first", "second"}, slice)
}

func TestIsMappingName(t *testing.T) {
	for _, name := range []string{
		"/memfd:OTEL_CTX",
		"/memfd:OTEL_CTX (deleted)",
		"[anon_shmem:OTEL_CTX]",
		"[anon:OTEL_CTX]",
	} {
		assert.True(t, IsMappingName(name), name)
	}

	for _, name := range []string{
		"",
		"/memfd:OTEL_CTX_BACKUP",
		"/memfd:datadog-tracer-info-abcd1234",
		"[anon:OTEL_CTX2]",
		"[heap]",
	} {
		assert.False(t, IsMappingName(name), name)
	}
}

// TestDecodeRejectsDeeplyNestedValue checks that the AnyValue/ArrayValue recursion is
// bounded, the payload being nested as deeply as the target process cares to.
func TestDecodeRejectsDeeplyNestedValue(t *testing.T) {
	nest := func(depth int) []byte {
		value := pbString("innermost")
		for range depth {
			value = pbBytes(fieldAnyValueArray, pbBytes(fieldArrayValueValues, value))
		}
		return value
	}

	ctx, err := decodeProcessContext(pbAttribute("shallow", nest(1)))
	require.NoError(t, err)
	slice, ok := ctx.Attributes.StringSlice("shallow")
	require.True(t, ok)
	assert.Equal(t, []string{"innermost"}, slice)

	_, err = decodeProcessContext(pbAttribute("deep", nest(10000)))
	assert.ErrorIs(t, err, errTooDeep)
}
