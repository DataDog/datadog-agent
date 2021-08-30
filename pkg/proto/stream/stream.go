package stream

import (
	"bytes"
	"io"
	"math"

	"google.golang.org/protobuf/encoding/protowire"
)

const (
	// wire types; see https://developers.google.com/protocol-buffers/docs/encoding
	wtVarint          int = 0
	wt64Bit           int = 1
	wtLengthDelimited int = 2
	wt32Bit           int = 5

	// defaultBufferSize is the default size for buffers used for embedded
	// values, which must first be written to a buffer to determine their
	// length.  This is not used if BufferFactory is set.
	defaultBufferSize int = 1024 * 16
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
	scratchBuffer []byte

	// scratchArray is a second, very small array used for packed encodings.  It is large
	// enough to fit two max-size varints (10 bytes each) without reallocation
	scratchArray [20]byte

	// childStream is a ProtoStream used to implement `Embedded`, and reused for
	// multiple calls.
	childStream *ProtoStream

	// childBuffer is the buffer to which `childStream` writes.
	childBuffer *bytes.Buffer

	// BufferFactory creates new, empty buffers as needed.  Users may override
	// this function to provide pre-initialized buffers of a larger size, or
	// from a buffer pool, for example.
	BufferFactory func() []byte
}

// NewProtoStream creates a new ProtoStream.  This ProtoStream _cannot be used_ until
// Reset is called.
func NewProtoStream() *ProtoStream {
	return &ProtoStream{
		scratchBuffer: make([]byte, 0, defaultBufferSize),
		childStream:   nil,
		childBuffer:   nil,
		BufferFactory: func() []byte { return make([]byte, 0, defaultBufferSize) },
	}
}

// Reset sets the Writer to which this ProtoStream streams.  This must be
// called before any other methods, and can be called again to change the
// output Writer.
func (ps *ProtoStream) Reset(outputWriter io.Writer) {
	ps.outputWriter = outputWriter
}

// Double writes a value of proto type double to the stream.
func (ps *ProtoStream) Double(fieldNumber int, value float64) error {
	if value == 0.0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, math.Float64bits(value))
	return ps.writeScratch()
}

// DoublePacked writes a slice of values of proto type double to the stream,
// in packed form.
func (ps *ProtoStream) DoublePacked(fieldNumber int, values []float64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, math.Float64bits(value))
	}

	return ps.writeScratchAsPacked(fieldNumber)
}

// Float writes a value of proto type double to the stream.
func (ps *ProtoStream) Float(fieldNumber int, value float32) error {
	if value == 0.0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, math.Float32bits(value))
	return ps.writeScratch()
}

// FloatPacked writes a slice of values of proto type float to the stream,
// in packed form.
func (ps *ProtoStream) FloatPacked(fieldNumber int, values []float32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, math.Float32bits(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Int32 writes a value of proto type int32 to the stream.
func (ps *ProtoStream) Int32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	return ps.writeScratch()
}

// Int32Packed writes a slice of values of proto type int32 to the stream,
// in packed form.
func (ps *ProtoStream) Int32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Int64 writes a value of proto type int64 to the stream.
func (ps *ProtoStream) Int64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	return ps.writeScratch()
}

// Int64Packed writes a slice of values of proto type int64 to the stream,
// in packed form.
func (ps *ProtoStream) Int64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Uint32 writes a value of proto type uint32 to the stream.
func (ps *ProtoStream) Uint32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	return ps.writeScratch()
}

// Uint32Packed writes a slice of values of proto type uint32 to the stream,
// in packed form.
func (ps *ProtoStream) Uint32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Uint64 writes a value of proto type uint64 to the stream.
func (ps *ProtoStream) Uint64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, value)
	return ps.writeScratch()
}

// Uint64Packed writes a slice of values of proto type uint64 to the stream,
// in packed form.
func (ps *ProtoStream) Uint64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, value)
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Sint32 writes a value of proto type sint32 to the stream.
func (ps *ProtoStream) Sint32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, zigzag32(uint64(value)))
	return ps.writeScratch()
}

// Sint32Packed writes a slice of values of proto type sint32 to the stream,
// in packed form.
func (ps *ProtoStream) Sint32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, zigzag32(uint64(value)))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Sint64 writes a value of proto type sint64 to the stream.
func (ps *ProtoStream) Sint64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, zigzag64(uint64(value)))
	return ps.writeScratch()
}

// Sint64Packed writes a slice of values of proto type sint64 to the stream,
// in packed form.
func (ps *ProtoStream) Sint64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, zigzag64(uint64(value)))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Fixed32 writes a value of proto type fixed32 to the stream.
func (ps *ProtoStream) Fixed32(fieldNumber int, value uint32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, value)
	return ps.writeScratch()
}

// Fixed32Packed writes a slice of values of proto type fixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed32Packed(fieldNumber int, values []uint32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, value)
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Fixed64 writes a value of proto type fixed64 to the stream.
func (ps *ProtoStream) Fixed64(fieldNumber int, value uint64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, value)
	return ps.writeScratch()
}

// Fixed64Packed writes a slice of values of proto type fixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Fixed64Packed(fieldNumber int, values []uint64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, value)
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Sfixed32 writes a value of proto type sfixed32 to the stream.
func (ps *ProtoStream) Sfixed32(fieldNumber int, value int32) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt32Bit)
	ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, uint32(value))
	return ps.writeScratch()
}

// Sfixed32Packed writes a slice of values of proto type sfixed32 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed32Packed(fieldNumber int, values []int32) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed32(ps.scratchBuffer, uint32(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Sfixed64 writes a value of proto type sfixed64 to the stream.
func (ps *ProtoStream) Sfixed64(fieldNumber int, value int64) error {
	if value == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wt64Bit)
	ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, uint64(value))
	return ps.writeScratch()
}

// Sfixed64Packed writes a slice of values of proto type sfixed64 to the stream,
// in packed form.
func (ps *ProtoStream) Sfixed64Packed(fieldNumber int, values []int64) error {
	if len(values) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	for _, value := range values {
		ps.scratchBuffer = protowire.AppendFixed64(ps.scratchBuffer, uint64(value))
	}
	return ps.writeScratchAsPacked(fieldNumber)
}

// Bool writes a value of proto type bool to the stream.
func (ps *ProtoStream) Bool(fieldNumber int, value bool) error {
	if value == false {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtVarint)
	var bit uint64
	if value {
		bit = 1
	}
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, bit)
	return ps.writeScratch()
}

// String writes a string to the stream.
func (ps *ProtoStream) String(fieldNumber int, value string) error {
	if len(value) == 0 {
		return nil
	}
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(len(value)))
	err := ps.writeScratch()
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
	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(len(value)))
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
	// create a new child, writing to a buffer, if one does not already exist
	if ps.childStream == nil {
		ps.childBuffer = bytes.NewBuffer(ps.BufferFactory())
		ps.childStream = NewProtoStream()
		ps.childStream.Reset(ps.childBuffer)
	}

	// write the embedded value using the child, leaving the result in ps.childBuffer
	ps.childBuffer.Reset()
	err := inner(ps.childStream)
	if err != nil {
		return err
	}

	ps.scratchBuffer = ps.scratchBuffer[:0]
	ps.encodeKeyToScratch(fieldNumber, wtLengthDelimited)
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(ps.childBuffer.Len()))

	// write the key and length prefix
	err = ps.writeScratch()
	if err != nil {
		return err
	}

	// and write out the embedded message
	return ps.writeAll(ps.childBuffer.Bytes())
}

// writeScratch flushes the scratch buffer to output
func (ps *ProtoStream) writeScratch() error {
	return ps.writeAll(ps.scratchBuffer)
}

// writeScratchAsPacked writes the scratch buffer to outputWriter, prefixed with
// the given key and the length of the scratch buffer.  This is used for packed
// encodings.
func (ps *ProtoStream) writeScratchAsPacked(fieldNumber int) error {
	// The scratch buffer is full of the packed data, but we need to write
	// the key and size, so we use scratchArray.  We could use a stack allocation
	// here, but as of writing the go compiler is not smart enough to figure out
	// that the value does not escape.
	keysize := ps.scratchArray[:0]
	keysize = protowire.AppendVarint(keysize, uint64((fieldNumber<<3)|wtLengthDelimited))
	keysize = protowire.AppendVarint(keysize, uint64(len(ps.scratchBuffer)))

	// write the key and length prefix
	err := ps.writeAll(keysize)
	if err != nil {
		return err
	}

	// write out the embedded message
	err = ps.writeScratch()
	if err != nil {
		return err
	}

	return nil
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
	ps.scratchBuffer = protowire.AppendVarint(ps.scratchBuffer, uint64(fieldNumber)<<3+uint64(wireType))
}

func zigzag32(v uint64) uint64 {
	return uint64((uint32(v) << 1) ^ uint32((int32(v) >> 31)))
}

func zigzag64(v uint64) uint64 {
	return (v << 1) ^ uint64((int64(v) >> 63))
}
