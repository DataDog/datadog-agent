package msrpc

import (
	"fmt"
	"io"
)

// InvalidResponseError identifies RPC decoding failures, separately from
// errors returned by the underlying pipe reader.
type InvalidResponseError struct{ Message string }

func (e *InvalidResponseError) Error() string {
	if e == nil {
		return "empty RPC response error"
	}
	return e.Message
}

// FaultError is a DCE/RPC fault returned instead of a response PDU.
type FaultError struct{ Status uint32 }

func (e *FaultError) Error() string {
	if e == nil {
		return "empty RPC fault"
	}
	return fmt.Sprintf("RPC fault 0x%08x", e.Status)
}

// ValidateBindAck checks the response to the requested bind, including the
// accepted transfer syntax. The transport remains owned by its caller.
func ValidateBindAck(packet []byte, callID uint32) error {
	ack := BindAckDecoder(packet)
	if ack.IsInvalid() || ack.CallId() != callID {
		return &InvalidResponseError{"broken bind ack response format"}
	}
	if !ack.AcceptsNDR() {
		return &InvalidResponseError{"bind ack did not accept NDR v2"}
	}
	return nil
}

// ReadStub assembles a complete RPC response from the initial pipe-transceive
// bytes and later pipe reads. The limit applies to the assembled NDR stub.
func ReadStub(initial []byte, callID uint32, limit int, read func(buffer []byte, minimum int) (int, error)) ([]byte, error) {
	if limit < 0 {
		return nil, &InvalidResponseError{"negative RPC response size limit"}
	}
	scratch := make([]byte, DefaultMaxFragmentSize)
	remaining := initial
	var output []byte
	first := true
	for {
		packet := remaining
		fill := func(minimum int) error {
			if read == nil {
				return io.ErrUnexpectedEOF
			}
			n, err := read(scratch, minimum)
			if err != nil {
				return err
			}
			if n < minimum || n > len(scratch) {
				return io.ErrUnexpectedEOF
			}
			packet = append(packet, scratch[:n]...)
			return nil
		}
		if len(packet) < HeaderSize {
			if err := fill(HeaderSize - len(packet)); err != nil {
				return nil, err
			}
		}
		hdr := ResponseHeaderDecoder(packet)
		if packet[2] == RPC_TYPE_FAULT {
			common := CommonHeaderDecoder(packet)
			length := int(common.FragLength())
			if common.IsInvalidCommon(HeaderSize) || length < 28 || length > DefaultMaxFragmentSize || common.CallId() != callID ||
				common.AuthLength() != 0 || packet[4] != 0x10 || packet[5] != 0 || packet[6] != 0 || packet[7] != 0 {
				return nil, &InvalidResponseError{"broken RPC fault response format"}
			}
			if len(packet) < length {
				if err := fill(length - len(packet)); err != nil {
					return nil, err
				}
			}
			if len(packet) != length {
				return nil, &InvalidResponseError{"data after RPC fault response"}
			}
			status := le.Uint32(packet[24:28])
			if status == 0 && length == 36 && le.Uint32(packet[16:20]) == 4 {
				// macOS returns the fault code in a four-byte stub.
				status = le.Uint32(packet[32:36])
			}
			return nil, &FaultError{Status: status}
		}
		if hdr.IsInvalid() || hdr.CallId() != callID ||
			packet[4] != 0x10 || packet[5] != 0 || packet[6] != 0 || packet[7] != 0 {
			return nil, &InvalidResponseError{"broken RPC response format"}
		}
		flags := hdr.PacketFlags()
		if (first && flags&RPC_PACKET_FLAG_FIRST == 0) || (!first && flags&RPC_PACKET_FLAG_FIRST != 0) {
			return nil, &InvalidResponseError{"invalid RPC response fragment flags"}
		}
		length := int(hdr.FragLength())
		if len(packet) < length {
			if err := fill(length - len(packet)); err != nil {
				return nil, err
			}
		}
		fragment := ResponseFragmentDecoder(packet[:length])
		if fragment.IsInvalid() || le.Uint16(packet[20:22]) != 0 {
			return nil, &InvalidResponseError{"broken RPC response format"}
		}
		remaining = packet[length:]
		chunk := fragment.Stub()
		if !first && len(chunk) == 0 {
			return nil, &InvalidResponseError{"empty RPC response fragment"}
		}
		if limit >= 0 && len(chunk) > limit-len(output) {
			return nil, &InvalidResponseError{"RPC response exceeds maximum size"}
		}
		output = append(output, chunk...)
		if flags&RPC_PACKET_FLAG_LAST != 0 {
			if len(remaining) != 0 {
				return nil, &InvalidResponseError{"data after final RPC response fragment"}
			}
			return output, nil
		}
		first = false
	}
}

// ReadShareNames assembles RPC response fragments and decodes the resulting
// NetrShareEnum stub. read must fill at least minimum bytes of buffer or return
// an error; it may return additional bytes up to len(buffer). A negative limit
// disables the aggregate stub-size limit. Pipe errors are returned unchanged.
func ReadShareNames(initial []byte, callID uint32, limit int, read func(buffer []byte, minimum int) (int, error)) ([]string, error) {
	scratch := make([]byte, DefaultMaxFragmentSize)
	remaining := initial
	var output []byte
	first := true
	for {
		packet := remaining
		fill := func(minimum int) error {
			if read == nil {
				return io.ErrUnexpectedEOF
			}
			n, err := read(scratch, minimum)
			if err != nil {
				return err
			}
			if n < minimum || n > len(scratch) {
				return io.ErrUnexpectedEOF
			}
			packet = append(packet, scratch[:n]...)
			return nil
		}
		if len(packet) < HeaderSize {
			if err := fill(HeaderSize - len(packet)); err != nil {
				return nil, err
			}
		}
		header := ResponseHeaderDecoder(packet)
		if packet[2] == RPC_TYPE_FAULT {
			_, err := ReadStub(packet, callID, 0, read)
			return nil, err
		}
		if header.IsInvalid() || header.CallId() != callID ||
			packet[4] != 0x10 || packet[5] != 0 || packet[6] != 0 || packet[7] != 0 {
			return nil, &InvalidResponseError{"broken net share enum response format"}
		}
		flags := header.PacketFlags()
		if (first && flags&RPC_PACKET_FLAG_FIRST == 0) || (!first && flags&RPC_PACKET_FLAG_FIRST != 0) {
			return nil, &InvalidResponseError{"invalid net share enum response fragment flags"}
		}
		length := int(header.FragLength())
		if len(packet) < length {
			if err := fill(length - len(packet)); err != nil {
				return nil, err
			}
		}
		fragment := ResponseFragmentDecoder(packet[:length])
		if fragment.IsInvalid() || le.Uint16(packet[20:22]) != 0 {
			return nil, &InvalidResponseError{"broken net share enum response format"}
		}
		remaining = packet[length:]
		chunk := fragment.Stub()
		if !first && len(chunk) == 0 {
			return nil, &InvalidResponseError{"empty net share enum response fragment"}
		}
		if limit >= 0 && len(chunk) > limit-len(output) {
			return nil, &InvalidResponseError{"net share enum response exceeds maximum size"}
		}
		output = append(output, chunk...)
		if flags&RPC_PACKET_FLAG_LAST != 0 {
			if len(remaining) != 0 {
				return nil, &InvalidResponseError{"broken net share enum response format"}
			}
			break
		}
		first = false
	}
	names, err := DecodeNetShareEnumAllShareNames(output)
	if err != nil {
		return nil, &InvalidResponseError{fmt.Sprintf("broken net share enum response format: %v", err)}
	}
	return names, nil
}
