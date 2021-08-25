package stream

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

const fieldDub int = 1
const fieldFlo int = 2
const fieldI32 = 3
const fieldI64 = 4
const fieldU32 = 5
const fieldU64 = 6
const fieldS32 = 7
const fieldS64 = 8
const fieldF32 = 9
const fieldF64 = 10
const fieldSF32 = 11
const fieldSF64 = 12
const fieldBoo = 13
const fieldStr = 14
const fieldByt = 15
const fieldEnu = 16

const enuE0 = 0
const enuE2 = 2

// Test that a simple message encoding properly decodes using the generated
// protofbuf code.  This function should test all proto types.
func TestSimpleEncoding(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	require.NoError(t, ps.Double(fieldDub, 3.14))
	require.NoError(t, ps.Float(fieldFlo, 3.14))
	require.NoError(t, ps.Int32(fieldI32, int32(-10)))
	require.NoError(t, ps.Int64(fieldI64, int64(-11)))
	require.NoError(t, ps.Uint32(fieldU32, uint32(12)))
	require.NoError(t, ps.Uint64(fieldU64, uint64(13)))
	require.NoError(t, ps.Sint32(fieldS32, int32(-12)))
	require.NoError(t, ps.Sint64(fieldS64, int64(-13)))
	require.NoError(t, ps.Fixed32(fieldF32, uint32(22)))
	require.NoError(t, ps.Fixed64(fieldF64, uint64(23)))
	require.NoError(t, ps.Sfixed32(fieldSF32, int32(-22)))
	require.NoError(t, ps.Sfixed64(fieldSF64, int64(-23)))
	require.NoError(t, ps.Bool(fieldBoo, true))
	require.NoError(t, ps.String(fieldStr, "s"))
	require.NoError(t, ps.Bytes(fieldByt, []byte("b")))
	require.NoError(t, ps.Int32(fieldEnu, enuE2))

	buf := output.Bytes()
	var res XmasTree

	require.NoError(t, proto.Unmarshal(buf, &res))

	assert.Equal(t, float64(3.14), res.Dub)
	assert.Equal(t, float32(3.14), res.Flo)
	assert.Equal(t, int32(-10), res.I32)
	assert.Equal(t, int64(-11), res.I64)
	assert.Equal(t, uint32(12), res.U32)
	assert.Equal(t, uint64(13), res.U64)
	assert.Equal(t, int32(-12), res.S32)
	assert.Equal(t, int64(-13), res.S64)
	assert.Equal(t, uint32(22), res.F32)
	assert.Equal(t, uint64(23), res.F64)
	assert.Equal(t, int32(-22), res.Sf32)
	assert.Equal(t, int64(-23), res.Sf64)
	assert.Equal(t, true, res.Boo)
	assert.Equal(t, "s", res.Str)
	assert.Equal(t, "b", string(res.Byt))
	assert.Equal(t, XmasTree_e2, res.Enu)
}

// Test that the zero values for each field do not result in any encoded data.
func TestSimpleEncodingZero(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	require.NoError(t, ps.Double(fieldDub, 0.0))
	require.NoError(t, ps.Float(fieldFlo, 0.0))
	require.NoError(t, ps.Int32(fieldI32, int32(0)))
	require.NoError(t, ps.Int64(fieldI64, int64(0)))
	require.NoError(t, ps.Uint32(fieldU32, uint32(0)))
	require.NoError(t, ps.Uint64(fieldU64, uint64(0)))
	require.NoError(t, ps.Sint32(fieldS32, int32(0)))
	require.NoError(t, ps.Sint64(fieldS64, int64(0)))
	require.NoError(t, ps.Fixed32(fieldF32, uint32(0)))
	require.NoError(t, ps.Fixed64(fieldF64, uint64(0)))
	require.NoError(t, ps.Sfixed32(fieldSF32, int32(0)))
	require.NoError(t, ps.Sfixed64(fieldSF64, int64(0)))
	require.NoError(t, ps.Bool(fieldBoo, false))
	require.NoError(t, ps.String(fieldStr, ""))
	require.NoError(t, ps.Bytes(fieldByt, []byte("")))
	require.NoError(t, ps.Int32(fieldEnu, enuE0))

	buf := output.Bytes()
	// all of those are zero values, so nothing should have been written
	require.Equal(t, 0, len(buf))
}

// Test that the zero values for packed fields do not result in any encoded data.
func TestPackedEncodingZero(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	require.NoError(t, ps.DoublePacked(fieldDub, []float64{}))
	require.NoError(t, ps.FloatPacked(fieldFlo, []float32{}))
	require.NoError(t, ps.Int32Packed(fieldI32, []int32{}))
	require.NoError(t, ps.Int64Packed(fieldI64, []int64{}))
	require.NoError(t, ps.Uint32Packed(fieldU32, []uint32{}))
	require.NoError(t, ps.Uint64Packed(fieldU64, []uint64{}))
	require.NoError(t, ps.Sint32Packed(fieldS32, []int32{}))
	require.NoError(t, ps.Sint64Packed(fieldS64, []int64{}))
	require.NoError(t, ps.Fixed32Packed(fieldF32, []uint32{}))
	require.NoError(t, ps.Fixed64Packed(fieldF64, []uint64{}))
	require.NoError(t, ps.Sfixed32Packed(fieldSF32, []int32{}))
	require.NoError(t, ps.Sfixed64Packed(fieldSF64, []int64{}))

	buf := output.Bytes()
	// all of those are zero values, so nothing should have been written
	require.Equal(t, 0, len(buf))
}

// Test that *Packed functions work.
func TestPacking(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	assertBytes := func(t *testing.T, exp []byte, got []byte) {
		assert.Equal(t, fmt.Sprintf("%#v", exp), fmt.Sprintf("%#v", got))
	}

	key := func(fieldNumber int) uint8 {
		return uint8((fieldNumber << 3) + wtLengthDelimited)
	}

	t.Run("DoublePacked", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.DoublePacked(fieldDub, []float64{3.14, 1.414}))
		assertBytes(t, []byte{key(fieldDub), 0x10, 0x1f, 0x85, 0xeb, 0x51, 0xb8, 0x1e, 0x9, 0x40, 0x39, 0xb4, 0xc8, 0x76, 0xbe, 0x9f, 0xf6, 0x3f}, output.Bytes())
	})

	t.Run("FloatPacked", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.FloatPacked(fieldFlo, []float32{3.14, 1.414}))
		assertBytes(t, []byte{key(fieldFlo), 0x8, 0xc3, 0xf5, 0x48, 0x40, 0xf4, 0xfd, 0xb4, 0x3f}, output.Bytes())
	})

	t.Run("Int32Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Int32Packed(fieldI32, []int32{int32(-12), int32(12), int32(-13)}))
		assertBytes(t, []byte{key(fieldI32), 0x15, 0xf4, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1, 0xc, 0xf3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1}, output.Bytes())
	})

	t.Run("Int64Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Int64Packed(fieldI64, []int64{int64(-12), int64(12), int64(-13)}))
		assertBytes(t, []byte{key(fieldI64), 0x15, 0xf4, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1, 0xc, 0xf3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1}, output.Bytes())
	})

	t.Run("Uint32Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Uint32Packed(fieldU32, []uint32{uint32(1), uint32(2), uint32(3)}))
		assertBytes(t, []byte{key(fieldU32), 0x3, 0x1, 0x2, 0x3}, output.Bytes())
	})

	t.Run("Uint64Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Uint64Packed(fieldU64, []uint64{uint64(1), uint64(2), uint64(3)}))
		assertBytes(t, []byte{key(fieldU64), 0x3, 0x1, 0x2, 0x3}, output.Bytes())
	})

	t.Run("Sint32Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Sint32Packed(fieldS32, []int32{int32(-12), int32(12), int32(-13)}))
		assertBytes(t, []byte{key(fieldS32), 0x03, 0x17, 0x18, 0x19}, output.Bytes())
	})

	t.Run("Sint64Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Sint64Packed(fieldS64, []int64{int64(-12), int64(12), int64(-13)}))
		assertBytes(t, []byte{key(fieldS64), 0x03, 0x17, 0x18, 0x19}, output.Bytes())
	})

	t.Run("Fixed32Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Fixed32Packed(fieldF32, []uint32{uint32(12), uint32(13), uint32(14)}))
		assertBytes(t, []byte{key(fieldF32), 0xc, 0xc, 0x0, 0x0, 0x0, 0xd, 0x0, 0x0, 0x0, 0xe, 0x0, 0x0, 0x0}, output.Bytes())
	})

	t.Run("Fixed64Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Fixed64Packed(fieldF64, []uint64{uint64(12), uint64(13), uint64(14)}))
		assertBytes(t, []byte{key(fieldF64), 0x18, 0xc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xd, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, output.Bytes())
	})

	t.Run("Sfixed32Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Sfixed32Packed(fieldSF32, []int32{int32(12), int32(-13), int32(14)}))
		assertBytes(t, []byte{key(fieldSF32), 0xc, 0xc, 0x0, 0x0, 0x0, 0xf3, 0xff, 0xff, 0xff, 0xe, 0x0, 0x0, 0x0}, output.Bytes())
	})

	t.Run("Sfixed64Packed", func(t *testing.T) {
		output.Reset()
		require.NoError(t, ps.Sfixed64Packed(fieldSF64, []int64{int64(12), int64(-13), int64(14)}))
		assertBytes(t, []byte{key(fieldSF64), 0x18, 0xc, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xf3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xe, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0}, output.Bytes())
	})
}

// Test using ps.EmbeddedMessage to embed a proto.Message instance
func TestEmbeddedMessage(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	const fieldAPIKey = 10
	const fieldRequest = 11

	require.NoError(t, ps.String(fieldAPIKey, "abc-123"))
	require.NoError(t, ps.EmbeddedMessage(fieldRequest, &SearchRequest{Query: "author=butler"}))
	require.NoError(t, ps.EmbeddedMessage(fieldRequest, &SearchRequest{Query: "author=rumi"}))

	buf := output.Bytes()
	var res MultiSearch

	require.NoError(t, proto.Unmarshal(buf, &res))

	require.Equal(t, "abc-123", res.ApiKey)
	require.Equal(t, "author=butler", res.Request[0].Query)
	require.Equal(t, int32(0), res.Request[0].PageNumber)
	require.Equal(t, "author=rumi", res.Request[1].Query)
	require.Equal(t, int32(0), res.Request[1].ResultPerPage)
}

// Test ps.Embedded embedding a repeated message
func TestEmbedding(t *testing.T) {
	output := bytes.NewBuffer([]byte{})
	ps := New(output)

	const fieldAPIKey = 10
	const fieldRequest = 11

	makeQuery := func(q string) func(*ProtoStream) error {
		return func(ps *ProtoStream) error {
			const fieldQuery int = 1
			const fieldPageNumber int = 2
			const fieldResultPerPage = 3
			var err error

			err = ps.String(fieldQuery, q)
			if err != nil {
				return err
			}
			err = ps.Int32(fieldPageNumber, 1)
			if err != nil {
				return err
			}
			err = ps.Int32(fieldResultPerPage, 10)
			if err != nil {
				return err
			}
			return nil
		}
	}

	require.NoError(t, ps.String(fieldAPIKey, "abc-123"))
	require.NoError(t, ps.Embedded(fieldRequest, makeQuery("author=butler")))
	require.NoError(t, ps.Embedded(fieldRequest, makeQuery("author=rumi")))

	buf := output.Bytes()
	var res MultiSearch

	require.NoError(t, proto.Unmarshal(buf, &res))

	require.Equal(t, "abc-123", res.ApiKey)
	require.Equal(t, "author=butler", res.Request[0].Query)
	require.Equal(t, int32(1), res.Request[0].PageNumber)
	require.Equal(t, int32(10), res.Request[0].ResultPerPage)
	require.Equal(t, "author=rumi", res.Request[1].Query)
	require.Equal(t, int32(1), res.Request[1].PageNumber)
	require.Equal(t, int32(10), res.Request[1].ResultPerPage)
}
