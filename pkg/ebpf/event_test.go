package ebpf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testConn = ConnectionStats{
		Pid:                123,
		Type:               1,
		Family:             0,
		Source:             "192.168.0.1",
		Dest:               "192.168.0.103",
		SPort:              123,
		DPort:              35000,
		MonotonicSentBytes: 123123,
		MonotonicRecvBytes: 312312,
	}
)

var sink string

func BenchmarkUniqueConnKeyString(b *testing.B) {
	c := testConn
	for n := 0; n < b.N; n++ {
		sink = fmt.Sprintf("%d-%d-%d-%s-%d-%s-%d", c.Pid, c.Type, c.Family, c.Source, c.SPort, c.Dest, c.DPort)
	}
	sink += ""
}

func BenchmarkUniqueConnKeyByteBuffer(b *testing.B) {
	c := testConn
	buf := new(bytes.Buffer)
	for n := 0; n < b.N; n++ {
		buf.Reset()
		buf.WriteString(c.Source)
		buf.WriteString(c.Dest)
		binary.Write(buf, binary.LittleEndian, c.Pid)
		binary.Write(buf, binary.LittleEndian, c.Type)
		binary.Write(buf, binary.LittleEndian, c.Family)
		binary.Write(buf, binary.LittleEndian, c.SPort)
		binary.Write(buf, binary.LittleEndian, c.DPort)
		buf.Bytes()
	}
}

func BenchmarkUniqueConnKeyByteBufferPacked(b *testing.B) {
	c := testConn
	buf := new(bytes.Buffer)
	for n := 0; n < b.N; n++ {
		buf.Reset()
		// PID (32 bits) + SPort (16 bits) + DPort (16 bits) = 64 bits
		p0 := uint64(c.Pid)<<32 | uint64(c.SPort)<<16 | uint64(c.DPort)
		binary.Write(buf, binary.LittleEndian, p0)
		buf.WriteString(c.Source)
		// Family (8 bits) + Type (8 bits) = 16 bits
		p1 := uint16(c.Family)<<8 | uint16(c.Type)
		binary.Write(buf, binary.LittleEndian, p1)
		buf.WriteString(c.Dest)
		buf.Bytes()
	}
}

func TestConnStatsByteKey(t *testing.T) {
	buf := new(bytes.Buffer)
	for _, test := range []struct {
		a ConnectionStats
		b ConnectionStats
	}{
		{
			a: ConnectionStats{Pid: 1},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{Family: 1},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{Type: 1},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{Source: "hello"},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{Dest: "goodbye"},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{SPort: 1},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{DPort: 1},
			b: ConnectionStats{},
		},
		{
			a: ConnectionStats{Pid: 1, Family: 0, Type: 1, Source: "a"},
			b: ConnectionStats{Pid: 1, Family: 0, Type: 1, Source: "b"},
		},
		{
			a: ConnectionStats{Pid: 1, Dest: "b", Family: 0, Type: 1, Source: "a"},
			b: ConnectionStats{Pid: 1, Dest: "a", Family: 0, Type: 1, Source: "b"},
		},
		{
			a: ConnectionStats{Pid: 1, Dest: "", Family: 0, Type: 1, Source: "a"},
			b: ConnectionStats{Pid: 1, Dest: "a", Family: 0, Type: 1, Source: ""},
		},
		{
			a: ConnectionStats{Pid: 1, Dest: "b", Family: 0, Type: 1},
			b: ConnectionStats{Pid: 1, Family: 0, Type: 1, Source: "b"},
		},
		{
			a: ConnectionStats{Pid: 1, Dest: "b", Family: 1},
			b: ConnectionStats{Pid: 1, Dest: "b", Type: 1},
		},
		{
			a: ConnectionStats{Pid: 1, Dest: "b", Type: 0, SPort: 3},
			b: ConnectionStats{Pid: 1, Dest: "b", Type: 0, DPort: 3},
		},
	} {
		var keyA, keyB string
		if b, err := test.a.ByteKey(buf); assert.NoError(t, err) {
			keyA = string(b)
		}
		if b, err := test.b.ByteKey(buf); assert.NoError(t, err) {
			keyB = string(b)
		}
		assert.NotEqual(t, keyA, keyB)
	}
}
