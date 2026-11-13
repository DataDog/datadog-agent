package msrpc

import (
	"bytes"
	"encoding/binary"
)

const (
	DefaultMaxFragmentSize = 4280
	HeaderSize             = 24

	RPC_VERSION       = 5
	RPC_VERSION_MINOR = 0

	RPC_TYPE_REQUEST  = 0
	RPC_TYPE_RESPONSE = 2
	RPC_TYPE_FAULT    = 3
	RPC_TYPE_BIND     = 11
	RPC_TYPE_BIND_ACK = 12

	RPC_PACKET_FLAG_FIRST = 0x01
	RPC_PACKET_FLAG_LAST  = 0x02

	SRVSVC_VERSION       = 3
	SRVSVC_VERSION_MINOR = 0
	LSARPC_VERSION       = 0

	NDR_VERSION = 2

	OP_NET_SHARE_ENUM     = 15
	OP_LSAR_CLOSE         = 0
	OP_LSAR_LOOKUP_SIDS   = 15
	OP_LSAR_OPEN_POLICY2  = 44
	OP_LSAR_GET_USER_NAME = 45
	OP_LSAR_LOOKUP_NAMES3 = 68
)

var (
	// SRVSVC UUID: 4B324FC8-1670-01D3-1278-5A47BF6EE188
	SRVSVC_UUID = [16]byte{0xc8, 0x4f, 0x32, 0x4b, 0x70, 0x16, 0xd3, 0x01, 0x12, 0x78, 0x5a, 0x47, 0xbf, 0x6e, 0xe1, 0x88}
	// LSARPC UUID: 12345778-1234-ABCD-EF00-0123456789AB
	LSARPC_UUID = [16]byte{0x78, 0x57, 0x34, 0x12, 0x34, 0x12, 0xcd, 0xab, 0xef, 0x00, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab}
	// NDR 32 Transfer Syntax UUID: 8A885D04-1CEB-11C9-9FE8-08002B104860
	NDR_UUID = [16]byte{0x04, 0x5d, 0x88, 0x8a, 0xeb, 0x1c, 0xc9, 0x11, 0x9f, 0xe8, 0x08, 0x00, 0x2b, 0x10, 0x48, 0x60}
)

var le = binary.LittleEndian

// encodeCommonHeader writes the 16-byte DCE/RPC connection-oriented common header.
func encodeCommonHeader(b []byte, ptype uint8, pfcFlags uint8, fragLen uint16, authLen uint16, callId uint32) {
	b[0] = RPC_VERSION
	b[1] = RPC_VERSION_MINOR
	b[2] = ptype
	b[3] = pfcFlags

	// Data Representation: Little-Endian (0x10), ASCII (0x00), IEEE (0x00), Reserved (0x00)
	b[4] = 0x10
	b[5] = 0x00
	b[6] = 0x00
	b[7] = 0x00

	le.PutUint16(b[8:10], fragLen)
	le.PutUint16(b[10:12], authLen)
	le.PutUint32(b[12:16], callId)
}

type CommonHeaderDecoder []byte

func (c CommonHeaderDecoder) IsInvalidCommon(minLen int) bool {
	if len(c) < minLen {
		return true
	}
	if c[0] != RPC_VERSION || c[1] != RPC_VERSION_MINOR {
		return true
	}
	return false
}

func (c CommonHeaderDecoder) Version() uint8 {
	return c[0]
}

func (c CommonHeaderDecoder) VersionMinor() uint8 {
	return c[1]
}

func (c CommonHeaderDecoder) PacketType() uint8 {
	return c[2]
}

func (c CommonHeaderDecoder) PacketFlags() uint8 {
	return c[3]
}

func (c CommonHeaderDecoder) DataRepresentation() []byte {
	return c[4:8]
}

func (c CommonHeaderDecoder) FragLength() uint16 {
	return le.Uint16(c[8:10])
}

func (c CommonHeaderDecoder) AuthLength() uint16 {
	return le.Uint16(c[10:12])
}

func (c CommonHeaderDecoder) CallId() uint32 {
	return le.Uint32(c[12:16])
}

// Bind represents an RPC bind request PDU.
type Bind struct {
	CallId         uint32
	AbstractSyntax [16]byte
	Version        uint16
}

// Call is one unfragmented DCE/RPC request PDU.
type Call struct {
	CallId uint32
	Opnum  uint16
	Stub   []byte
}

func (r *Call) Size() int { return HeaderSize + len(r.Stub) }

func (r *Call) Encode(b []byte) {
	encodeCommonHeader(b, RPC_TYPE_REQUEST, RPC_PACKET_FLAG_FIRST|RPC_PACKET_FLAG_LAST, uint16(r.Size()), 0, r.CallId)
	le.PutUint32(b[16:20], uint32(len(r.Stub)))
	le.PutUint16(b[20:22], 0)
	le.PutUint16(b[22:24], r.Opnum)
	copy(b[HeaderSize:], r.Stub)
}

func (r *Bind) Size() int {
	return 72
}

func (r *Bind) Encode(b []byte) {
	encodeCommonHeader(b, RPC_TYPE_BIND, RPC_PACKET_FLAG_FIRST|RPC_PACKET_FLAG_LAST, 72, 0, r.CallId)

	le.PutUint16(b[16:18], DefaultMaxFragmentSize) // max_xmit_frag
	le.PutUint16(b[18:20], DefaultMaxFragmentSize) // max_recv_frag
	le.PutUint32(b[20:24], 0)                      // assoc_group_id
	le.PutUint32(b[24:28], 1)                      // n_context_elem = 1
	le.PutUint16(b[28:30], 0)                      // context_id = 0
	le.PutUint16(b[30:32], 1)                      // n_transfer_syn = 1

	// Abstract Syntax
	copy(b[32:48], r.AbstractSyntax[:])
	le.PutUint16(b[48:50], r.Version)
	le.PutUint16(b[50:52], 0)

	// Transfer Syntax (NDR v2.0)
	copy(b[52:68], NDR_UUID[:])
	le.PutUint32(b[68:72], NDR_VERSION)
}

// BindAckDecoder decodes an RPC bind_ack response PDU.
type BindAckDecoder []byte

func (c BindAckDecoder) IsInvalid() bool {
	hdr := CommonHeaderDecoder(c)
	if hdr.IsInvalidCommon(26) {
		return true
	}
	if hdr.PacketType() != RPC_TYPE_BIND_ACK {
		return true
	}
	// Only support a complete, unauthenticated, little-endian bind_ack PDU
	// using the [C706] 12.6 layout referenced by [MS-RPCE] 3.3.1.5.6.
	if int(hdr.FragLength()) != len(c) || c[4] != 0x10 ||
		hdr.PacketFlags()&(RPC_PACKET_FLAG_FIRST|RPC_PACKET_FLAG_LAST) != RPC_PACKET_FLAG_FIRST|RPC_PACKET_FLAG_LAST ||
		hdr.AuthLength() != 0 {
		return true
	}
	secAddrEnd := 26 + int(le.Uint16(c[24:26]))
	if secAddrEnd > len(c) {
		return true
	}
	resultList := (secAddrEnd + 3) &^ 3
	if resultList+4 > len(c) {
		return true
	}
	return resultList+4+int(c[resultList])*24 != len(c)
}

// AcceptsNDR reports whether the single context proposed by Bind was accepted.
func (c BindAckDecoder) AcceptsNDR() bool {
	if c.IsInvalid() {
		return false
	}
	resultList := (26 + int(le.Uint16(c[24:26])) + 3) &^ 3
	// [MS-RPCE] 3.3.1.5.6 requires results to match the proposed contexts
	// in count and order, and calls to fail if no transfer syntax is accepted.
	if c[resultList] != 1 {
		return false
	}
	result := c[resultList+4:]
	return le.Uint16(result[:2]) == 0 && bytes.Equal(result[4:20], NDR_UUID[:]) &&
		le.Uint32(result[20:24]) == NDR_VERSION
}

func (c BindAckDecoder) Version() uint8 {
	return CommonHeaderDecoder(c).Version()
}

func (c BindAckDecoder) VersionMinor() uint8 {
	return CommonHeaderDecoder(c).VersionMinor()
}

func (c BindAckDecoder) PacketType() uint8 {
	return CommonHeaderDecoder(c).PacketType()
}

func (c BindAckDecoder) PacketFlags() uint8 {
	return CommonHeaderDecoder(c).PacketFlags()
}

func (c BindAckDecoder) DataRepresentation() []byte {
	return CommonHeaderDecoder(c).DataRepresentation()
}

func (c BindAckDecoder) FragLength() uint16 {
	return CommonHeaderDecoder(c).FragLength()
}

func (c BindAckDecoder) AuthLength() uint16 {
	return CommonHeaderDecoder(c).AuthLength()
}

func (c BindAckDecoder) CallId() uint32 {
	return CommonHeaderDecoder(c).CallId()
}

func (c BindAckDecoder) MaxXmitFrag() uint16 {
	return le.Uint16(c[16:18])
}

func (c BindAckDecoder) MaxRecvFrag() uint16 {
	return le.Uint16(c[18:20])
}

func (c BindAckDecoder) AssocGroupId() uint32 {
	return le.Uint32(c[20:24])
}

// ResponseHeaderDecoder reads an RPC response header, possibly before its body arrives.
type ResponseHeaderDecoder []byte

func (c ResponseHeaderDecoder) IsInvalid() bool {
	hdr := CommonHeaderDecoder(c)
	if hdr.IsInvalidCommon(HeaderSize) || hdr.PacketType() != RPC_TYPE_RESPONSE {
		return true
	}
	if hdr.AuthLength() != 0 {
		return true
	}
	return hdr.FragLength() < HeaderSize || hdr.FragLength() > DefaultMaxFragmentSize
}

func (c ResponseHeaderDecoder) FragLength() uint16 { return CommonHeaderDecoder(c).FragLength() }
func (c ResponseHeaderDecoder) CallId() uint32     { return CommonHeaderDecoder(c).CallId() }
func (c ResponseHeaderDecoder) PacketFlags() uint8 { return CommonHeaderDecoder(c).PacketFlags() }

// ResponseFragmentDecoder reads exactly one complete RPC response fragment.
// Call IsInvalid before accessing its header fields or stub.
type ResponseFragmentDecoder []byte

func (c ResponseFragmentDecoder) Header() ResponseHeaderDecoder { return ResponseHeaderDecoder(c) }

func (c ResponseFragmentDecoder) IsInvalid() bool {
	header := c.Header()
	return header.IsInvalid() || int(header.FragLength()) != len(c)
}

// Stub returns the fragment body without its RPC header.
func (c ResponseFragmentDecoder) Stub() []byte { return c[HeaderSize:] }
