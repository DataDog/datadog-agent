package stream

import (
	"bytes"
	"io"
	"log"
	"math"

	"github.com/golang/protobuf/proto"
)

const (
	// wire types; see https://developers.google.com/protocol-buffers/docs/encoding
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
	// outputWriter is the writer to which the protobuf-encoded bytes are written
	outputWriter io.Writer

	// scratchBuffer is a buffer used and re-used for generating output.  Each method
	// should begin by resetting this buffer.
	scratchBuffer *proto.Buffer

	// childStream is a ProtoStream used to implement `Embedded`, and reused for
	// multiple calls.
	childStream *ProtoStream

	// childBuffer is the buffer to which `childStream` writes.
	childBuffer bytes.Buffer
}

// NewProtoStream creates a new ProtoStream, ready to write encoded data to the embedded
// writer.
func NewProtoStream(output io.Writer) *ProtoStream {
	return &ProtoStream{
		outputWriter:  output,
		scratchBuffer: proto.NewBuffer([]byte{}),
		childStream:   nil,
		childBuffer:   bytes.Buffer{},
	}
}

// Double writes a value of proto type double to the stream.
func (ps *ProtoStream) Double(fieldNumber int, value float64) error {
	if value == 0.0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	err := ps.scratchBuffer.EncodeFixed64(math.Float64bits(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// DoublePacked writes a slice of values of proto type double to the stream,
// in packed form.
func (ps *ProtoStream) DoublePacked(fieldNumber int, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	log.Printf("values %#v", values)
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed64(math.Float64bits(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Float writes a value of proto type double to the stream.
func (ps *ProtoStream) Float(fieldNumber int, value float32) error {
	if value == 0.0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	err := ps.scratchBuffer.EncodeFixed32(uint64(math.Float32bits(value)))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// FloatPacked writes a slice of values of proto type float to the stream,
// in packed form.
func (ps *ProtoStream) FloatPacked(fieldNumber int, values []float32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed32(uint64(math.Float32bits(value)))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Int32 writes a value of proto type int32 to the stream.
func (ps *ProtoStream) Int32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeVarint(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Int32Packed writes a slice of values of proto type int32 to the stream,
// in packed form.
func (ps *ProtoStream) Int32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeVarint(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Int64 writes a value of proto type int64 to the stream.
func (ps *ProtoStream) Int64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeVarint(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Int64Packed writes a slice of values of proto type int64 to the stream,
// in packed form.
func (ps *ProtoStream) Int64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeVarint(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Uint32 writes a value of proto type uint32 to the stream.
func (ps *ProtoStream) Uint32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeVarint(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Uint32Packed writes a slice of values of proto type uint32 to the stream,
// in packed form.
func (ps *ProtoStream) Uint32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeVarint(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Uint64 writes a value of proto type uint64 to the stream.
func (ps *ProtoStream) Uint64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeVarint(value)
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Uint64Packed writes a slice of values of proto type uint64 to the stream,
// in packed form.
func (ps *ProtoStream) Uint64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeVarint(value)
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Sint32 writes a value of proto type sint32 to the stream.
func (ps *ProtoStream) Sint32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeZigzag32(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Sint32Packed writes a slice of values of proto type sint32 to the stream,
// in packed form.
func (ps *ProtoStream) Sint32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeZigzag32(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Sint64 writes a value of proto type sint64 to the stream.
func (ps *ProtoStream) Sint64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	err := ps.scratchBuffer.EncodeZigzag64(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Sint64Packed writes a slice of values of proto type sint64 to the stream,
// in packed form.
func (ps *ProtoStream) Sint64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeZigzag64(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Fixed32 writes a value of proto type fixed32 to the stream.
func (ps *ProtoStream) Fixed32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	err := ps.scratchBuffer.EncodeFixed32(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Fixed32Packed writes a slice of values of proto type fixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed32(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Fixed64 writes a value of proto type fixed64 to the stream.
func (ps *ProtoStream) Fixed64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	err := ps.scratchBuffer.EncodeFixed64(value)
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Fixed64Packed writes a slice of values of proto type fixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed64(value)
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Sfixed32 writes a value of proto type sfixed32 to the stream.
func (ps *ProtoStream) Sfixed32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	err := ps.scratchBuffer.EncodeFixed32(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Sfixed32Packed writes a slice of values of proto type sfixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed32(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Sfixed64 writes a value of proto type sfixed64 to the stream.
func (ps *ProtoStream) Sfixed64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	err := ps.scratchBuffer.EncodeFixed64(uint64(value))
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// Sfixed64Packed writes a slice of values of proto type sfixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	return ps.Embedded(fieldNumber, func(ps *ProtoStream) error {
		ps.scratchBuffer.Reset()
		for _, value := range values {
			err := ps.scratchBuffer.EncodeFixed64(uint64(value))
			if err != nil {
				return err
			}
		}
		return ps.writeScratch()
	})
}

// Bool writes a value of proto type bool to the stream.
func (ps *ProtoStream) Bool(fieldNumber int, value bool) error {
	if value == false {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	var bit uint64
	if value {
		bit = 1
	}
	err := ps.scratchBuffer.EncodeVarint(bit)
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// String writes a string to the stream.
func (ps *ProtoStream) String(fieldNumber int, value string) error {
	if len(value) == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	err := ps.scratchBuffer.EncodeVarint(uint64(len(value)))
	if err != nil {
		return err
	}
	err = ps.writeScratch()
	if err != nil {
		return err
	}

	return ps.writeAllString(value)
}

// Bytes writes the given bytes to the stream.
func (ps *ProtoStream) Bytes(fieldNumber int, value []byte) error {
	if len(value) == 0 {
		return nil
	}
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	err := ps.scratchBuffer.EncodeVarint(uint64(len(value)))
	if err != nil {
		return err
	}
	err = ps.writeScratch()
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
	if ps.childStream == nil {
		ps.childStream = NewProtoStream(&ps.childBuffer)
	}

	// write the embedded value using the child, leaving the result in ps.childBuffer
	ps.childBuffer.Reset()
	err := inner(ps.childStream)
	if err != nil {
		return err
	}

	log.Printf("child buffer: %#v", ps.childBuffer.Bytes())
	log.Printf("child buffer len: 0x%x", ps.childBuffer.Len())

	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	err = ps.scratchBuffer.EncodeVarint(uint64(ps.childBuffer.Len()))
	if err != nil {
		return err
	}

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
	ps.scratchBuffer.Reset()
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	err := ps.scratchBuffer.EncodeMessage(m)
	if err != nil {
		return err
	}
	return ps.writeScratch()
}

// writeScratch flushes the scratch buffer to output
func (ps *ProtoStream) writeScratch() error {
	return ps.writeAll(ps.scratchBuffer.Bytes())
}

// writeAll writes an entire buffer to output
func (ps *ProtoStream) writeAll(buf []byte) error {
	for len(buf) > 0 {
		n, err := ps.outputWriter.Write(buf)
		if err != nil {
			return err
		}
		buf = buf[n:]
	}
	return nil
}

// writeAllString writes an entire string to output, using io.WriteString
// to avoid allocation
func (ps *ProtoStream) writeAllString(value string) error {
	for len(value) > 0 {
		n, err := io.WriteString(ps.outputWriter, value)
		if err != nil {
			return err
		}
		value = value[n:]
	}
	return nil
}

// encodeKeyToScratch encodes a protobuf key into ps.scratch
func (ps *ProtoStream) encodeKeyToScratch(fieldNumber int, wireType int) {
	// field/wireType are always a valid varint
	_ = ps.scratchBuffer.EncodeVarint(uint64(fieldNumber)<<3 + uint64(wireType))
}
