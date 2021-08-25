package stream

import (
	"bytes"
	"io"
	"math"

	"github.com/golang/protobuf/proto"
)

const (
	wtVarint          int = 0
	wt64Bit           int = 1
	wtLengthDelimited int = 2
	wt32Bit           int = 5
)

// A ProtoStream supports writing protobuf data in a streaming fashion.  Its methods
// will write their output to the wrapped `io.Writer`.  Zero values are not included.
//
// ProtoStream instances are *not* threadsafe and *not* re-entrant.
type ProtoStream struct {
	// output is the writer to which the protobuf-encoded bytes are written
	output io.Writer

	// scratch is a buffer used and re-used for generating output.  Each method
	// should begin by resetting this buffer.
	scratch *proto.Buffer

	// child is a ProtoStream used to implement `Embedded`, and reused for
	// multiple calls.
	child *ProtoStream

	// childBuffer is the buffer to which `child` writes.
	childBuffer bytes.Buffer
}

// New creates a new ProtoStream, ready to write encoded data to the embedded
// writer.
func New(output io.Writer) *ProtoStream {
	return &ProtoStream{
		output:      output,
		scratch:     proto.NewBuffer([]byte{}),
		child:       nil,
		childBuffer: bytes.Buffer{},
	}
}

// Double writes a value of proto type double to the stream.
func (ps *ProtoStream) Double(fieldNumber int, value float64) error {
	if value == 0.0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratch.EncodeFixed64(math.Float64bits(value))
	return ps.writeScratch()
}

// DoublePacked writes a slice of values of proto type double to the stream,
// in packed form.
func (ps *ProtoStream) DoublePacked(fieldNumber int, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed64(math.Float64bits(value))
		}
		return ps.writeScratch()
	})
}

// Float writes a value of proto type double to the stream.
func (ps *ProtoStream) Float(fieldNumber int, value float32) error {
	if value == 0.0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratch.EncodeFixed32(uint64(math.Float32bits(value)))
	return ps.writeScratch()
}

// FloatPacked writes a slice of values of proto type float to the stream,
// in packed form.
func (ps *ProtoStream) FloatPacked(fieldNumber int, values []float32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed32(uint64(math.Float32bits(value)))
		}
		return ps.writeScratch()
	})
}

// Int32 writes a value of proto type int32 to the stream.
func (ps *ProtoStream) Int32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeVarint(uint64(value))
	return ps.writeScratch()
}

// Int32Packed writes a slice of values of proto type int32 to the stream,
// in packed form.
func (ps *ProtoStream) Int32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeVarint(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Int64 writes a value of proto type int64 to the stream.
func (ps *ProtoStream) Int64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeVarint(uint64(value))
	return ps.writeScratch()
}

// Int64Packed writes a slice of values of proto type int64 to the stream,
// in packed form.
func (ps *ProtoStream) Int64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeVarint(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Uint32 writes a value of proto type uint32 to the stream.
func (ps *ProtoStream) Uint32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeVarint(uint64(value))
	return ps.writeScratch()
}

// Uint32Packed writes a slice of values of proto type uint32 to the stream,
// in packed form.
func (ps *ProtoStream) Uint32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeVarint(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Uint64 writes a value of proto type uint64 to the stream.
func (ps *ProtoStream) Uint64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeVarint(uint64(value))
	return ps.writeScratch()
}

// Uint64Packed writes a slice of values of proto type uint64 to the stream,
// in packed form.
func (ps *ProtoStream) Uint64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeVarint(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Sint32 writes a value of proto type sint32 to the stream.
func (ps *ProtoStream) Sint32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeZigzag32(uint64(value))
	return ps.writeScratch()
}

// Sint32Packed writes a slice of values of proto type sint32 to the stream,
// in packed form.
func (ps *ProtoStream) Sint32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeZigzag32(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Sint64 writes a value of proto type sint64 to the stream.
func (ps *ProtoStream) Sint64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratch.EncodeZigzag64(uint64(value))
	return ps.writeScratch()
}

// Sint64Packed writes a slice of values of proto type sint64 to the stream,
// in packed form.
func (ps *ProtoStream) Sint64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeZigzag64(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Fixed32 writes a value of proto type fixed32 to the stream.
func (ps *ProtoStream) Fixed32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratch.EncodeFixed32(uint64(value))
	return ps.writeScratch()
}

// Fixed32Packed writes a slice of values of proto type fixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed32(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Fixed64 writes a value of proto type fixed64 to the stream.
func (ps *ProtoStream) Fixed64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratch.EncodeFixed64(value)
	return ps.writeScratch()
}

// Fixed64Packed writes a slice of values of proto type fixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed64(value)
		}
		return ps.writeScratch()
	})
}

// Sfixed32 writes a value of proto type sfixed32 to the stream.
func (ps *ProtoStream) Sfixed32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratch.EncodeFixed32(uint64(value))
	return ps.writeScratch()
}

// Sfixed32Packed writes a slice of values of proto type sfixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed32(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Sfixed64 writes a value of proto type sfixed64 to the stream.
func (ps *ProtoStream) Sfixed64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratch.EncodeFixed64(uint64(value))
	return ps.writeScratch()
}

// Sfixed64Packed writes a slice of values of proto type sfixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratch.Reset()
		for _, value := range values {
			ps.scratch.EncodeFixed64(uint64(value))
		}
		return ps.writeScratch()
	})
}

// Bool writes a value of proto type bool to the stream.
func (ps *ProtoStream) Bool(fieldNumber int, value bool) error {
	if value == false {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	var bit uint64
	if value {
		bit = 1
	}
	ps.scratch.EncodeVarint(bit)
	return ps.writeScratch()
}

// String writes a string to the stream.
//
// NOTE: the string must be copied to convert it to []byte, so where possible
// prefer the Bytes method.
func (ps *ProtoStream) String(fieldNumber int, value string) error {
	if value == "" {
		return nil
	}
	return ps.Bytes(fieldNumber, []byte(value))
}

// Bytes writes the given bytes to the stream.
func (ps *ProtoStream) Bytes(fieldNumber int, value []byte) error {
	if len(value) == 0 {
		return nil
	}
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratch.EncodeVarint(uint64(len(value)))
	err := ps.writeScratch()
	if err != nil {
		return err
	}

	return ps.writeAll(value)
}

// Embedded is used for constructing embedded messages.  It calls the given
// function with a new ProtoStream, then embeds the result in the current
// stream.
//
// NOTE: if the inner function creates an empty message (such as for a struct
// at its zero value), that empty message will still be added to the stream.
func (ps *ProtoStream) Embedded(fieldNumber int, inner func(*ProtoStream) error) error {
	if ps.child == nil {
		ps.child = New(&ps.childBuffer)
	}

	// write the embedded value using the child, leaving the result in ps.childBuffer
	ps.childBuffer.Reset()
	err := inner(ps.child)
	if err != nil {
		return err
	}

	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratch.EncodeVarint(uint64(ps.childBuffer.Len()))

	// write the key and length prefix
	err = ps.writeScratch()
	if err != nil {
		return err
	}

	// and write out the embedded message
	return ps.writeAll(ps.childBuffer.Bytes())
}

// EmbeddedMessage is similar to Embedded, but embeds a proto.Message directly.
func (ps *ProtoStream) EmbeddedMessage(fieldNumber int, m proto.Message) error {
	ps.scratch.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratch.EncodeMessage(m)
	return ps.writeScratch()
}

// writeScratch flushes the scratch buffer to output
func (ps *ProtoStream) writeScratch() error {
	return ps.writeAll(ps.scratch.Bytes())
}

// writeAll writes an entire buffer to output
func (ps *ProtoStream) writeAll(buf []byte) error {
	for len(buf) > 0 {
		n, err := ps.output.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

// encodeKeyToScratch encodes a protobuf key into ps.scratch
func (ps *ProtoStream) encodeKeyToScratch(fieldNumber int, wireType int) {
	ps.scratch.EncodeVarint(uint64(fieldNumber)<<3 + uint64(wireType))
}
