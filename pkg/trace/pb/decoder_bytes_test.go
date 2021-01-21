package pb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tinylib/msgp/msgp"
)

func TestParseFloat64Bytes(t *testing.T) {
	assert := assert.New(t)

	data := []byte{
		0x2a,             // 42
		0xd1, 0xfb, 0x2e, // -1234
		0xcd, 0x0a, 0x9b, // 2715
		0xcb, 0x40, 0x09, 0x1e, 0xb8, 0x51, 0xeb, 0x85, 0x1f, // float64(3.14)
	}

	var (
		f   float64
		err error
	)
	bts := data

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(42.0, f)

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(-1234.0, f)

	f, bts, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(2715.0, f)

	f, _, err = parseFloat64Bytes(bts)
	assert.NoError(err)
	assert.Equal(3.14, f)
}

func TestDecodeBytes(t *testing.T) {
	want := Traces{
		{{Service: "A", Name: "op"}},
		{{Service: "B"}},
		{{Service: "C"}},
	}
	var (
		bts []byte
		err error
	)
	if bts, err = want.MarshalMsg(nil); err != nil {
		t.Fatal(err)
	}
	var got Traces
	if _, err = got.UnmarshalMsg(bts); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, want, got)
}

func TestDecodeInvalidUTF8Bytes(t *testing.T) {
	provide := Traces{
		{{Service: "A", Name: "op\x99\xbf"}},
		{{Service: "B"}},
		{{Service: "C"}},
	}
	accept := Traces{
		{{Service: "A", Name: "op��"}},
		{{Service: "B"}},
		{{Service: "C"}},
	}
	var (
		bts []byte
		err error
	)
	if bts, err = provide.MarshalMsg(nil); err != nil {
		t.Fatal(err)
	}
	var got Traces
	if _, err = got.UnmarshalMsg(bts); err != nil {
		t.Fatal(err)
	}
	assert.ElementsMatch(t, accept, got)
}

func TestDecodeNilStringInMap(t *testing.T) {
	var o []byte
	o = msgp.AppendMapHeader(o, uint32(1))
	// append meta
	o = append(o, 0xa4, 0x6d, 0x65, 0x74, 0x61)
	o = msgp.AppendMapHeader(o, 3)
	o = msgp.AppendNil(o)
	o = msgp.AppendString(o, "val1")
	o = msgp.AppendString(o, "key2")
	o = msgp.AppendNil(o)
	o = msgp.AppendString(o, "val3")
	o = msgp.AppendString(o, "key3")

	var s Span
	_, err := s.UnmarshalMsg(o)
	assert.Nil(t, err)
	assert.Equal(t, s.Meta, map[string]string{"": "val1", "key2": "", "val3": "key3"})
}
