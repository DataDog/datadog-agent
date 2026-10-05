package wire

import (
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

// ----------------------------------------------------------------------------
// SMB2 Error Response
//

type ErrorResponse struct {
	PacketHeader

	CommandCode Command
	ErrorData   Encoder // ErrorContextListResponse | (SymbolicLinkErrorResponse | SmallBufferErrorResponse)
}

func (c *ErrorResponse) Command() Command {
	return c.CommandCode
}

func (c *ErrorResponse) CreditCharge() uint16 {
	return 1
}

func (c *ErrorResponse) SetCreditCharge(u uint16) {}

func (c *ErrorResponse) Size() int {
	if c.ErrorData == nil {
		return 64 + 8 + 1
	}
	return 64 + 8 + c.ErrorData.Size()
}

func (c *ErrorResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 9) // StructureSize
	if c.ErrorData != nil {
		le.PutUint32(res[4:8], uint32(c.ErrorData.Size()))
		c.ErrorData.Encode(res[8:])

		if e, ok := c.ErrorData.(ErrorContextListResponse); ok {
			res[2] = uint8(len(e))
		}
	}
}

type ErrorResponseDecoder []byte

func (r ErrorResponseDecoder) IsInvalid() bool {
	if len(r) < 8 {
		return true
	}

	if r.StructureSize() != 9 {
		return true
	}

	if uint64(len(r)) < 8+uint64(r.ByteCount()) {
		return true
	}

	return false
}

func (r ErrorResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r ErrorResponseDecoder) ErrorContextCount() uint8 {
	return r[2]
}

func (r ErrorResponseDecoder) ByteCount() uint32 {
	return le.Uint32(r[4:8])
}

func (r ErrorResponseDecoder) ErrorData() []byte {
	return r[8 : 8+r.ByteCount()]
}

// ----------------------------------------------------------------------------
// SMB2 Error Context Response
//

// for SMB311

type ErrorContextListResponse []*ErrorContextResponse

func (c ErrorContextListResponse) Size() int {
	size := 0
	for _, ec := range c {
		size = Roundup(size, 8)
		size += ec.Size()
	}
	return size
}

func (c ErrorContextListResponse) Encode(p []byte) {
	off := 0
	for _, ec := range c {
		off = Roundup(off, 8)

		ec.Encode(p[off:])

		off += ec.Size()
	}
}

type ErrorContextResponse struct {
	ErrorId uint32

	ErrorData Encoder
}

func (c *ErrorContextResponse) Size() int {
	return 8 + c.ErrorData.Size()
}

func (c *ErrorContextResponse) Encode(p []byte) {
	le.PutUint32(p[:4], uint32(c.ErrorData.Size()))
	le.PutUint32(p[4:8], c.ErrorId)
	if c.ErrorData != nil {
		c.ErrorData.Encode(p[8:])
	}
}

type ErrorContextResponseDecoder []byte

func (ctx ErrorContextResponseDecoder) IsInvalid() bool {
	if len(ctx) < 8 {
		return true
	}

	if uint64(len(ctx)) < 8+uint64(ctx.ErrorDataLength()) {
		return true
	}

	return false
}

func (ctx ErrorContextResponseDecoder) ErrorDataLength() uint32 {
	return le.Uint32(ctx[:4])
}

func (ctx ErrorContextResponseDecoder) ErrorId() uint32 {
	return le.Uint32(ctx[4:8])
}

func (ctx ErrorContextResponseDecoder) ErrorData() []byte {
	return ctx[8 : 8+ctx.ErrorDataLength()]
}

func (ctx ErrorContextResponseDecoder) Next() int {
	return 8 + Roundup(int(ctx.ErrorDataLength()), 8)
}

// ----------------------------------------------------------------------------
// SMB2 ErrorData formats
//

type SmallBufferErrorResponse struct {
	RequiredBufferLength uint32
}

func (c *SmallBufferErrorResponse) Size() int {
	return 4
}

func (c *SmallBufferErrorResponse) Encode(p []byte) {
	le.PutUint32(p[:4], c.RequiredBufferLength)
}

type SmallBufferErrorResponseDecoder []byte

func (r SmallBufferErrorResponseDecoder) IsInvalid() bool {
	return len(r) != 4
}

func (r SmallBufferErrorResponseDecoder) RequiredBufferLength() uint32 {
	return le.Uint32(r)
}

type SymbolicLinkErrorResponse struct {
	UnparsedPathLength uint16
	Flags              uint32
	SubstituteName     string
	PrintName          string
}

func (c *SymbolicLinkErrorResponse) Size() int {
	return 28 + utf16le.EncodedStringLen(c.SubstituteName) + utf16le.EncodedStringLen(c.PrintName)
}

func (c *SymbolicLinkErrorResponse) Encode(p []byte) {
	slen := utf16le.EncodeString(p[28:], c.SubstituteName)
	plen := utf16le.EncodeString(p[28+slen:], c.PrintName)

	le.PutUint32(p[:4], uint32(c.Size()-4)) // SymLinkLength
	le.PutUint32(p[4:8], 0x4c4d5953)
	le.PutUint32(p[8:12], IO_REPARSE_TAG_SYMLINK)
	le.PutUint16(p[14:16], c.UnparsedPathLength)
	le.PutUint32(p[24:28], c.Flags)
	le.PutUint16(p[12:14], uint16(c.Size()-16)) // ReparseDataLength
	le.PutUint16(p[16:18], 0)                   // SubstituteNameOffset
	le.PutUint16(p[18:20], uint16(slen))        // SubstituteNameLength
	le.PutUint16(p[20:22], uint16(slen))        // PrintNameOffset
	le.PutUint16(p[22:24], uint16(plen))        // PrintNameLength
}

type SymbolicLinkErrorResponseDecoder []byte

func (r SymbolicLinkErrorResponseDecoder) IsInvalid() bool {
	if len(r) < 28 {
		return true
	}

	if r.SymLinkErrorTag() != 0x4c4d5953 {
		return true
	}

	if r.ReparseTag() != IO_REPARSE_TAG_SYMLINK {
		return true
	}

	tlen := uint64(r.SymLinkLength())
	rlen := uint64(r.ReparseDataLength())
	soff := uint64(r.SubstituteNameOffset())
	slen := uint64(r.SubstituteNameLength())
	poff := uint64(r.PrintNameOffset())
	plen := uint64(r.PrintNameLength())

	// These fields are byte lengths or offsets for UTF-16LE Unicode strings;
	// string lengths must therefore be even ([MS-SMB2] 2.2.2.2.1;
	// [MS-DTYP] 1.1).
	if (soff&1 | poff&1 | slen&1 | plen&1 | uint64(r.UnparsedPathLength())&1) != 0 {
		return true
	}

	if uint64(len(r)) < 4+tlen {
		return true
	}

	if tlen < 12+rlen {
		return true
	}

	if rlen < 12+soff+slen || rlen < 12+poff+plen {
		return true
	}

	pathBuffer := r.PathBuffer()
	substituteName := pathBuffer[soff : soff+slen]
	if isInvalidUTF16LE(substituteName) {
		return true
	}
	// [MS-SMB2] 2.2.2.2.1:
	// "For an absolute target that is on a remote machine, the server MUST
	// return the path in the format "\\?\UNC\server\share\..."..."
	// "The server SHOULD NOT return symbolic link information with an
	// absolute target that is a local resource, because local evaluation
	// will vary based on client operating system (OS).<7>"
	// Therefore, absolute targets without a UNC prefix (e.g. local drive
	// paths) are rejected as invalid.
	if r.Flags()&SYMLINK_FLAG_RELATIVE == 0 && !HasUNCPrefix(substituteName) {
		return true
	}
	if isInvalidSubstituteName(substituteName, r.Flags()) {
		return true
	}
	if plen > 0 && isInvalidUTF16LE(pathBuffer[poff:poff+plen]) {
		return true
	}

	return false
}

func (r SymbolicLinkErrorResponseDecoder) SymLinkLength() uint32 {
	return le.Uint32(r[:4])
}

func (r SymbolicLinkErrorResponseDecoder) SymLinkErrorTag() uint32 {
	return le.Uint32(r[4:8])
}

func (r SymbolicLinkErrorResponseDecoder) ReparseTag() uint32 {
	return le.Uint32(r[8:12])
}

func (r SymbolicLinkErrorResponseDecoder) ReparseDataLength() uint16 {
	return le.Uint16(r[12:14])
}

func (r SymbolicLinkErrorResponseDecoder) UnparsedPathLength() uint16 {
	return le.Uint16(r[14:16])
}

func (r SymbolicLinkErrorResponseDecoder) SubstituteNameOffset() uint16 {
	return le.Uint16(r[16:18])
}

func (r SymbolicLinkErrorResponseDecoder) SubstituteNameLength() uint16 {
	return le.Uint16(r[18:20])
}

func (r SymbolicLinkErrorResponseDecoder) PrintNameOffset() uint16 {
	return le.Uint16(r[20:22])
}

func (r SymbolicLinkErrorResponseDecoder) PrintNameLength() uint16 {
	return le.Uint16(r[22:24])
}

func (r SymbolicLinkErrorResponseDecoder) Flags() uint32 {
	return le.Uint32(r[24:28])
}

func (r SymbolicLinkErrorResponseDecoder) PathBuffer() []byte {
	return r[28:]
}

func (r SymbolicLinkErrorResponseDecoder) SubstituteName() string {
	off := int(r.SubstituteNameOffset())
	length := int(r.SubstituteNameLength())
	buf := r.PathBuffer()
	return normalizeSymlinkTarget(utf16le.DecodeToString(buf[off : off+length]))
}

func (r SymbolicLinkErrorResponseDecoder) PrintName() string {
	off := int(r.PrintNameOffset())
	length := int(r.PrintNameLength())
	buf := r.PathBuffer()
	return utf16le.DecodeToString(buf[off : off+length])
}

func (r SymbolicLinkErrorResponseDecoder) SplitUnparsedPath(name string) (string, string) {
	ws := UTF16FromString(name)
	ulen := int(r.UnparsedPathLength())
	if ulen/2 > len(ws) {
		return "", ""
	}

	return UTF16ToString(ws[:len(ws)-ulen/2]), UTF16ToString(ws[len(ws)-ulen/2:])
}

// ----------------------------------------------------------------------------
// SMB2 NEGOTIATE Response
//

type NegotiateResponse struct {
	PacketHeader

	SecurityMode    uint16
	DialectRevision uint16
	ServerGuid      uuid.UUID
	Capabilities    uint32
	MaxTransactSize uint32
	MaxReadSize     uint32
	MaxWriteSize    uint32
	SystemTime      Filetime
	ServerStartTime Filetime
	SecurityBuffer  []byte

	Contexts NegotiateContexts
}

func (c *NegotiateResponse) Command() Command {
	return SMB2_NEGOTIATE
}

func (c *NegotiateResponse) CreditCharge() uint16 {
	return 1
}

func (c *NegotiateResponse) SetCreditCharge(u uint16) {}

func (c *NegotiateResponse) Size() int {
	size := 64 + len(c.SecurityBuffer)
	if len(c.Contexts) > 0 {
		size = Roundup(size, 8) + c.Contexts.Size()
	}

	if size == 64 {
		return 64 + 64 + 1
	}

	return 64 + size
}

func (c *NegotiateResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 65) // StructureSize
	le.PutUint16(res[2:4], c.SecurityMode)
	le.PutUint16(res[4:6], c.DialectRevision)
	encodeGUID(c.ServerGuid, res[8:24])
	le.PutUint32(res[24:28], c.Capabilities)
	le.PutUint32(res[28:32], c.MaxTransactSize)
	le.PutUint32(res[32:36], c.MaxReadSize)
	le.PutUint32(res[36:40], c.MaxWriteSize)
	c.SystemTime.Encode(res[40:48])
	c.ServerStartTime.Encode(res[48:56])

	// SecurityBuffer
	{
		copy(res[64:], c.SecurityBuffer)
		le.PutUint16(res[56:58], 64+64)                         // SecurityBufferOffset
		le.PutUint16(res[58:60], uint16(len(c.SecurityBuffer))) // SecurityBufferLength
	}

	if len(c.Contexts) > 0 {
		off := Roundup(64+len(c.SecurityBuffer), 8)

		le.PutUint32(res[60:64], uint32(off+64)) // NegotiateContextOffset

		c.Contexts.Encode(res[off:])
	}

	le.PutUint16(res[6:8], uint16(len(c.Contexts))) // NegotiateContextCount
}

type NegotiateResponseDecoder []byte

func (r NegotiateResponseDecoder) IsInvalid() bool {
	if len(r) < 64 {
		return true
	}

	if r.StructureSize() != 65 {
		return true
	}

	packetLength := uint64(len(r)) + 64
	securityBufferOffset := uint64(r.SecurityBufferOffset())
	securityBufferLength := uint64(r.SecurityBufferLength())

	if r.DialectRevision() != SMB311 {
		// [MS-SMB2] 2.2.4 places the variable-length SecurityBuffer after the
		// 64-byte SMB2 header and 64-byte response structure, so a non-empty
		// buffer must start at offset 128 or later.
		if securityBufferLength != 0 && securityBufferOffset < 128 {
			return true
		}
		return packetLength < securityBufferOffset+securityBufferLength
	}

	// [MS-SMB2] 2.2.4 places negotiate contexts after the 64-byte SMB2 header
	// and 64-byte response structure (offset 128), aligned to 8 bytes,
	// following any non-empty security buffer.
	contextStart := uint64(128)
	if securityBufferLength != 0 {
		if securityBufferOffset < contextStart || securityBufferOffset > packetLength ||
			securityBufferLength > packetLength-securityBufferOffset {
			return true
		}
		contextStart = securityBufferOffset + securityBufferLength
	}

	negotiateContextOffset := uint64(r.NegotiateContextOffset())
	if negotiateContextOffset != 0 {
		if negotiateContextOffset < contextStart || negotiateContextOffset > packetLength ||
			negotiateContextOffset&7 != 0 {
			return true
		}
	} else if r.NegotiateContextCount() != 0 {
		return true
	}

	if count := r.NegotiateContextCount(); count > 0 {
		list := NegotiateContextsDecoder(r[int(negotiateContextOffset)-64:])
		if list.IsInvalid() || list.Count() != int(count) {
			return true
		}
	}

	return false
}

func (r NegotiateResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r NegotiateResponseDecoder) SecurityMode() uint16 {
	return le.Uint16(r[2:4])
}

func (r NegotiateResponseDecoder) DialectRevision() uint16 {
	return le.Uint16(r[4:6])
}

func (r NegotiateResponseDecoder) ServerGuid() uuid.UUID {
	return decodeGUID(r[8:24])
}

func (r NegotiateResponseDecoder) Capabilities() uint32 {
	return le.Uint32(r[24:28])
}

func (r NegotiateResponseDecoder) MaxTransactSize() uint32 {
	return le.Uint32(r[28:32])
}

func (r NegotiateResponseDecoder) MaxReadSize() uint32 {
	return le.Uint32(r[32:36])
}

func (r NegotiateResponseDecoder) MaxWriteSize() uint32 {
	return le.Uint32(r[36:40])
}

func (r NegotiateResponseDecoder) SystemTime() FiletimeDecoder {
	return FiletimeDecoder(r[40:48])
}

func (r NegotiateResponseDecoder) ServerStartTime() FiletimeDecoder {
	return FiletimeDecoder(r[48:56])
}

func (r NegotiateResponseDecoder) SecurityBufferOffset() uint16 {
	return le.Uint16(r[56:58])
}

func (r NegotiateResponseDecoder) SecurityBufferLength() uint16 {
	return le.Uint16(r[58:60])
}

func (r NegotiateResponseDecoder) SecurityBuffer() []byte {
	n := int(r.SecurityBufferLength())
	if n == 0 {
		return nil
	}
	off := int(r.SecurityBufferOffset()) - 64
	return r[off : off+n]
}

// From SMB311

func (r NegotiateResponseDecoder) NegotiateContextCount() uint16 {
	return le.Uint16(r[6:8])
}

func (r NegotiateResponseDecoder) NegotiateContextOffset() uint32 {
	return le.Uint32(r[60:64])
}

// Contexts is empty for dialects older than SMB 3.1.1, where
// NegotiateContextOffset is reserved and MUST be ignored ([MS-SMB2] 2.2.4).
func (r NegotiateResponseDecoder) Contexts() NegotiateContextsDecoder {
	if r.DialectRevision() != SMB311 {
		return nil
	}
	off := r.NegotiateContextOffset()
	if off == 0 || r.NegotiateContextCount() == 0 {
		return nil
	}
	return NegotiateContextsDecoder(r[off-64:])
}

// ----------------------------------------------------------------------------
// SMB2 SESSION_SETUP Response
//

type SessionSetupResponse struct {
	PacketHeader

	SessionFlags   uint16
	SecurityBuffer []byte
}

func (c *SessionSetupResponse) Command() Command {
	return SMB2_SESSION_SETUP
}

func (c *SessionSetupResponse) CreditCharge() uint16 {
	return 1
}

func (c *SessionSetupResponse) SetCreditCharge(u uint16) {}

func (c *SessionSetupResponse) Size() int {
	if len(c.SecurityBuffer) == 0 {
		return 64 + 8 + 1
	}

	return 64 + 8 + len(c.SecurityBuffer)
}

func (c *SessionSetupResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 9) // StructureSize
	le.PutUint16(res[2:4], c.SessionFlags)

	if len(c.SecurityBuffer) != 0 {
		le.PutUint16(res[4:6], 8+64) // SecurityBufferOffset

		copy(res[8:], c.SecurityBuffer)

		le.PutUint16(res[6:8], uint16(len(c.SecurityBuffer)))
	}
}

type SessionSetupResponseDecoder []byte

func (r SessionSetupResponseDecoder) IsInvalid() bool {
	if len(r) < 8 {
		return true
	}

	if r.StructureSize() != 9 {
		return true
	}

	// [MS-SMB2] 2.2.6: the variable-length Buffer follows the 64-byte SMB2
	// header and the 8-byte fixed response fields, so a non-empty security
	// buffer cannot start before offset 72.
	if r.SecurityBufferLength() > 0 {
		if r.SecurityBufferOffset() < 72 || uint64(len(r))+64 < uint64(r.SecurityBufferOffset())+uint64(r.SecurityBufferLength()) {
			return true
		}
	}

	return false
}

func (r SessionSetupResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r SessionSetupResponseDecoder) SessionFlags() uint16 {
	return le.Uint16(r[2:4])
}

func (r SessionSetupResponseDecoder) SecurityBufferOffset() uint16 {
	return le.Uint16(r[4:6])
}

func (r SessionSetupResponseDecoder) SecurityBufferLength() uint16 {
	return le.Uint16(r[6:8])
}

func (r SessionSetupResponseDecoder) SecurityBuffer() []byte {
	n := int(r.SecurityBufferLength())
	if n == 0 {
		return nil
	}
	off := int(r.SecurityBufferOffset()) - 64
	return r[off : off+n]
}

// ----------------------------------------------------------------------------
// SMB2 LOGOFF Response
//

type LogoffResponse struct {
	PacketHeader
}

func (c *LogoffResponse) Command() Command {
	return SMB2_LOGOFF
}

func (c *LogoffResponse) CreditCharge() uint16 {
	return 1
}

func (c *LogoffResponse) SetCreditCharge(u uint16) {}

func (c *LogoffResponse) Size() int {
	return 64 + 4
}

func (c *LogoffResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 4) // StructureSize
}

type LogoffResponseDecoder []byte

func (r LogoffResponseDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r LogoffResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 ECHO Response
//

type EchoResponse struct {
	PacketHeader
}

func (c *EchoResponse) Command() Command {
	return SMB2_ECHO
}

func (c *EchoResponse) CreditCharge() uint16 {
	return 1
}

func (c *EchoResponse) SetCreditCharge(u uint16) {}

func (c *EchoResponse) Size() int {
	return 64 + 4
}

func (c *EchoResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 4) // StructureSize
}

type EchoResponseDecoder []byte

func (r EchoResponseDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r EchoResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 TREE_CONNECT Response
//

type TreeConnectResponse struct {
	PacketHeader

	ShareType     uint8
	ShareFlags    uint32
	Capabilities  uint32
	MaximalAccess uint32
}

func (c *TreeConnectResponse) Command() Command {
	return SMB2_TREE_CONNECT
}

func (c *TreeConnectResponse) CreditCharge() uint16 {
	return 1
}

func (c *TreeConnectResponse) SetCreditCharge(u uint16) {}

func (c *TreeConnectResponse) Size() int {
	return 64 + 16
}

func (c *TreeConnectResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 16) // StructureSize
	res[2] = c.ShareType
	le.PutUint32(res[4:8], c.ShareFlags)
	le.PutUint32(res[8:12], c.Capabilities)
	le.PutUint32(res[12:16], c.MaximalAccess)
}

type TreeConnectResponseDecoder []byte

func (r TreeConnectResponseDecoder) IsInvalid() bool {
	if len(r) < 16 {
		return true
	}

	if r.StructureSize() != 16 {
		return true
	}

	// [MS-SMB2] 2.2.10: ShareType MUST be SMB2_SHARE_TYPE_DISK (0x01),
	// SMB2_SHARE_TYPE_PIPE (0x02), or SMB2_SHARE_TYPE_PRINT (0x03).
	switch r.ShareType() {
	case SMB2_SHARE_TYPE_DISK, SMB2_SHARE_TYPE_PIPE, SMB2_SHARE_TYPE_PRINT:
		return false
	default:
		return true
	}
}

func (r TreeConnectResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r TreeConnectResponseDecoder) ShareType() uint8 {
	return r[2]
}

func (r TreeConnectResponseDecoder) ShareFlags() uint32 {
	return le.Uint32(r[4:8])
}

func (r TreeConnectResponseDecoder) Capabilities() uint32 {
	return le.Uint32(r[8:12])
}

func (r TreeConnectResponseDecoder) MaximalAccess() uint32 {
	return le.Uint32(r[12:16])
}

// ----------------------------------------------------------------------------
// SMB2 TREE_DISCONNECT Response
//

type TreeDisconnectResponse struct {
	PacketHeader
}

func (c *TreeDisconnectResponse) Command() Command {
	return SMB2_TREE_DISCONNECT
}

func (c *TreeDisconnectResponse) CreditCharge() uint16 {
	return 1
}

func (c *TreeDisconnectResponse) SetCreditCharge(u uint16) {}

func (c *TreeDisconnectResponse) Size() int {
	return 64 + 4
}

func (c *TreeDisconnectResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 4) // StructureSize
}

type TreeDisconnectResponseDecoder []byte

func (r TreeDisconnectResponseDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r TreeDisconnectResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 CREATE Response
//

type CreateResponse struct {
	PacketHeader

	OplockLevel    uint8
	Flags          uint8
	CreateAction   uint32
	CreationTime   Filetime
	LastAccessTime Filetime
	LastWriteTime  Filetime
	ChangeTime     Filetime
	AllocationSize int64
	EndofFile      int64
	FileAttributes uint32
	FileId         FileId

	Contexts CreateContexts
}

func (c *CreateResponse) Command() Command {
	return SMB2_CREATE
}

func (c *CreateResponse) CreditCharge() uint16 {
	return 1
}

func (c *CreateResponse) SetCreditCharge(u uint16) {}

func (c *CreateResponse) Size() int {
	if len(c.Contexts) == 0 {
		return 64 + 88 + 1
	}

	return Roundup(64+88, 8) + c.Contexts.Size()
}

func (c *CreateResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 89) // StructureSize
	res[2] = c.OplockLevel
	res[3] = c.Flags
	le.PutUint32(res[4:8], c.CreateAction)
	c.CreationTime.Encode(res[8:16])
	c.LastAccessTime.Encode(res[16:24])
	c.LastWriteTime.Encode(res[24:32])
	c.ChangeTime.Encode(res[32:40])
	le.PutUint64(res[40:48], uint64(c.AllocationSize))
	le.PutUint64(res[48:56], uint64(c.EndofFile))
	le.PutUint32(res[56:60], c.FileAttributes)
	c.FileId.Encode(res[64:80])

	if len(c.Contexts) > 0 {
		off := Roundup(88, 8)

		le.PutUint32(res[80:84], uint32(64+off))            // CreateContextsOffset
		le.PutUint32(res[84:88], uint32(c.Contexts.Size())) // CreateContextsLength

		c.Contexts.Encode(res[off:])
	}
}

type CreateResponseDecoder []byte

func (r CreateResponseDecoder) IsInvalid() bool {
	if len(r) < 88 {
		return true
	}

	if r.StructureSize() != 89 {
		return true
	}

	// [MS-FSCC] 2.4.47 defines EndOfFile and AllocationSize as signed 64-bit
	// integers and mandates that EndOfFile MUST be >= 0. Negative values are
	// wire corruption or integer overflows that would corrupt file offsets
	// and stat sizes. Note that [MS-SMB2] 3.3.5.9 notes <322> and <324> state
	// that Windows servers set these fields to "any value" for named pipes
	// (rather than strictly 0 as recommended by the spec), but Windows NPFS
	// returns non-negative buffer metrics, never negative integers.
	if r.EndofFile() < 0 || r.AllocationSize() < 0 {
		return true
	}

	// [MS-SMB2] 2.2.14 specifies CreateAction MUST be one of FILE_SUPERSEDED (0),
	// FILE_OPENED (1), FILE_CREATED (2), or FILE_OVERWRITTEN (3).
	if r.CreateAction() > FILE_OVERWRITTEN {
		return true
	}

	for _, timestamp := range []FiletimeDecoder{
		r.CreationTime(),
		r.LastAccessTime(),
		r.LastWriteTime(),
		r.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}

	coff := r.CreateContextsOffset()
	clen := r.CreateContextsLength()

	// A non-empty Buffer must begin after the fixed 88-byte response
	// structure, while no contexts require a zero offset ([MS-SMB2] 2.2.14).
	if clen == 0 {
		if coff != 0 {
			return true
		}
	} else if coff < 64+88 {
		return true
	}

	if coff&7 != 0 {
		return true
	}

	if uint64(len(r))+64 < uint64(coff)+uint64(clen) {
		return true
	}

	if clen > 0 {
		if CreateContextsDecoder(r[int(coff)-64 : int(coff)-64+int(clen)]).IsInvalid() {
			return true
		}
		for _, context := range r.Contexts().Contexts() {
			if data := createContextData(context, "QFid"); data != nil && QueryOnDiskIDResponseDecoder(data).IsInvalid() {
				return true
			}
		}
	}

	return false
}

// QueryOnDiskID returns the validated QFid response data, or nil if the
// server did not return that context. It shares the CREATE response lifetime.
func (r CreateResponseDecoder) QueryOnDiskID() QueryOnDiskIDResponseDecoder {
	if r.CreateContextsLength() == 0 {
		return nil
	}
	for _, context := range r.Contexts().Contexts() {
		if data := createContextData(context, "QFid"); data != nil {
			return QueryOnDiskIDResponseDecoder(data)
		}
	}
	return nil
}

func (r CreateResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r CreateResponseDecoder) OplockLevel() uint8 {
	return r[2]
}

func (r CreateResponseDecoder) Flags() uint8 {
	return r[3]
}

func (r CreateResponseDecoder) CreateAction() uint32 {
	return le.Uint32(r[4:8])
}

func (r CreateResponseDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(r[8:16])
}

func (r CreateResponseDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(r[16:24])
}

func (r CreateResponseDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(r[24:32])
}

func (r CreateResponseDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(r[32:40])
}

func (r CreateResponseDecoder) AllocationSize() int64 {
	return int64(le.Uint64(r[40:48]))
}

func (r CreateResponseDecoder) EndofFile() int64 {
	return int64(le.Uint64(r[48:56]))
}

func (r CreateResponseDecoder) FileAttributes() uint32 {
	return le.Uint32(r[56:60])
}

func (r CreateResponseDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[64:80])
}

func (r CreateResponseDecoder) CreateContextsOffset() uint32 {
	return le.Uint32(r[80:84])
}

func (r CreateResponseDecoder) CreateContextsLength() uint32 {
	return le.Uint32(r[84:88])
}

func (r CreateResponseDecoder) Contexts() CreateContextsDecoder {
	length := int(r.CreateContextsLength())
	if length == 0 {
		return nil
	}
	off := int(r.CreateContextsOffset()) - 64
	return CreateContextsDecoder(r[off : off+length])
}

// ----------------------------------------------------------------------------
// SMB2 CLOSE Response
//

type CloseResponse struct {
	PacketHeader

	Flags          uint16
	CreationTime   Filetime
	LastAccessTime Filetime
	LastWriteTime  Filetime
	ChangeTime     Filetime
	AllocationSize int64
	EndofFile      int64
	FileAttributes uint32
}

func (c *CloseResponse) Command() Command {
	return SMB2_CLOSE
}

func (c *CloseResponse) CreditCharge() uint16 {
	return 1
}

func (c *CloseResponse) SetCreditCharge(u uint16) {}

func (c *CloseResponse) Size() int {
	return 64 + 60
}

func (c *CloseResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 60) // StructureSize
	le.PutUint16(res[2:4], c.Flags)
	c.CreationTime.Encode(res[8:16])
	c.LastAccessTime.Encode(res[16:24])
	c.LastWriteTime.Encode(res[24:32])
	c.ChangeTime.Encode(res[32:40])
	le.PutUint64(res[40:48], uint64(c.AllocationSize))
	le.PutUint64(res[48:56], uint64(c.EndofFile))
	le.PutUint32(res[56:60], c.FileAttributes)
}

type CloseResponseDecoder []byte

func (r CloseResponseDecoder) IsInvalid() bool {
	if len(r) < 60 {
		return true
	}

	if r.StructureSize() != 60 {
		return true
	}

	// [MS-SMB2] 2.2.16 specifies that AllocationSize and EndOfFile are attribute
	// values when closed (or zero if SMB2_CLOSE_FLAG_POSTQUERY_ATTRIB is not set).
	// [MS-FSCC] 2.4.47 defines both as signed 64-bit integers and mandates that
	// they MUST be >= 0. Negative values indicate wire corruption.
	if r.EndofFile() < 0 || r.AllocationSize() < 0 {
		return true
	}

	for _, timestamp := range []FiletimeDecoder{
		r.CreationTime(),
		r.LastAccessTime(),
		r.LastWriteTime(),
		r.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}

	return false
}

func (r CloseResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r CloseResponseDecoder) Flags() uint16 {
	return le.Uint16(r[2:4])
}

func (r CloseResponseDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(r[8:16])
}

func (r CloseResponseDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(r[16:24])
}

func (r CloseResponseDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(r[24:32])
}

func (r CloseResponseDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(r[32:40])
}

func (r CloseResponseDecoder) AllocationSize() int64 {
	return int64(le.Uint64(r[40:48]))
}

func (r CloseResponseDecoder) EndofFile() int64 {
	return int64(le.Uint64(r[48:56]))
}

func (r CloseResponseDecoder) FileAttributes() uint32 {
	return le.Uint32(r[56:60])
}

// ----------------------------------------------------------------------------
// SMB2 FLUSH Response
//

type FlushResponse struct {
	PacketHeader
}

func (c *FlushResponse) Command() Command {
	return SMB2_FLUSH
}

func (c *FlushResponse) CreditCharge() uint16 {
	return 1
}

func (c *FlushResponse) SetCreditCharge(u uint16) {}

func (c *FlushResponse) Size() int {
	return 64 + 4
}

func (c *FlushResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 4) // StructureSize
}

type FlushResponseDecoder []byte

func (r FlushResponseDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r FlushResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 READ Response
//

type ReadResponse struct {
	PacketHeader

	Data          []byte
	DataRemaining uint32
}

func (c *ReadResponse) Command() Command {
	return SMB2_READ
}

func (c *ReadResponse) CreditCharge() uint16 {
	return 1
}

func (c *ReadResponse) SetCreditCharge(u uint16) {}

func (c *ReadResponse) Size() int {
	if len(c.Data) == 0 {
		return 64 + 16 + 1
	}
	return 64 + 16 + len(c.Data)
}

func (c *ReadResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 17) // StructureSize
	res[2] = 64 + 16          // DataOffset
	copy(res[16:], c.Data)
	le.PutUint32(res[4:8], uint32(len(c.Data))) // DataLength
	le.PutUint32(res[8:12], c.DataRemaining)
}

type ReadResponseDecoder []byte

func (r ReadResponseDecoder) IsInvalidHeader() bool {
	if len(r) < 16 {
		return true
	}

	if r.StructureSize() != 17 {
		return true
	}

	if r.DataOffset() < 16+64 {
		return true
	}

	return false
}

func (r ReadResponseDecoder) IsInvalidPayload() bool {
	// [MS-SMB2] 2.2.20 requires at least one data byte on success; a read
	// returning zero bytes must use a STATUS_END_OF_FILE error response.
	return r.DataLength() == 0 || uint64(len(r))+64 < uint64(r.DataOffset())+uint64(r.DataLength())
}

func (r ReadResponseDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r ReadResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r ReadResponseDecoder) DataOffset() uint8 {
	return r[2]
}

func (r ReadResponseDecoder) DataLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r ReadResponseDecoder) DataRemaining() uint32 {
	return le.Uint32(r[8:12])
}

func (r ReadResponseDecoder) Flags() uint32 {
	return le.Uint32(r[12:16])
}

func (r ReadResponseDecoder) Data() []byte {
	off := int(r.DataOffset()) - 64
	return r[off : off+int(r.DataLength())]
}

// ----------------------------------------------------------------------------
// SMB2 WRITE Response
//

type WriteResponse struct {
	PacketHeader

	Count     uint32
	Remaining uint32
}

func (c *WriteResponse) Command() Command {
	return SMB2_WRITE
}

func (c *WriteResponse) CreditCharge() uint16 {
	return 1
}

func (c *WriteResponse) SetCreditCharge(u uint16) {}

func (c *WriteResponse) Size() int {
	return 64 + 16 + 1
}

func (c *WriteResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 17) // StructureSize
	le.PutUint32(res[4:8], c.Count)
	le.PutUint32(res[8:12], c.Remaining)
}

type WriteResponseDecoder []byte

func (r WriteResponseDecoder) IsInvalid() bool {
	if len(r) < 16 {
		return true
	}

	if r.StructureSize() != 17 {
		return true
	}

	return false
}

func (r WriteResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r WriteResponseDecoder) Count() uint32 {
	return le.Uint32(r[4:8])
}

func (r WriteResponseDecoder) Remaining() uint32 {
	return le.Uint32(r[8:12])
}

func (r WriteResponseDecoder) WriteChannelInfoOffset() uint16 {
	return le.Uint16(r[12:14])
}

func (r WriteResponseDecoder) WriteChannelInfoLength() uint16 {
	return le.Uint16(r[14:16])
}

// ----------------------------------------------------------------------------
// SMB2 OPLOCK_BREAK Notification and Response
//

// ----------------------------------------------------------------------------
// SMB2 LOCK Response
//

type LockResponse struct {
	PacketHeader
}

func (c *LockResponse) Command() Command {
	return SMB2_LOCK
}

func (c *LockResponse) CreditCharge() uint16 {
	return 1
}

func (c *LockResponse) SetCreditCharge(u uint16) {}

func (c *LockResponse) Size() int {
	return 64 + 4
}

func (c *LockResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 4) // StructureSize ([MS-SMB2] 2.2.27)
}

type LockResponseDecoder []byte

func (r LockResponseDecoder) IsInvalid() bool {
	return len(r) < 4 || r.StructureSize() != 4
}

func (r LockResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 IOCTL Response
//

type IoctlResponse struct {
	PacketHeader

	CtlCode uint32
	FileId  FileId
	Flags   uint32
	Input   Encoder
	Output  Encoder
}

func (c *IoctlResponse) Command() Command {
	return SMB2_IOCTL
}

func (c *IoctlResponse) CreditCharge() uint16 {
	return 1
}

func (c *IoctlResponse) SetCreditCharge(u uint16) {}

func (c *IoctlResponse) Size() int {
	size := 64 + 48
	if c.Input != nil {
		size += c.Input.Size()
	}
	if c.Output != nil {
		size = Roundup(size, 8) + c.Output.Size()
	}
	if size == 64+48 {
		return 64 + 48 + 1
	}
	return size
}

func (c *IoctlResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 49) // StructureSize
	le.PutUint32(res[4:8], c.CtlCode)
	c.FileId.Encode(res[8:24])
	le.PutUint32(res[40:44], c.Flags)

	off := 48
	// [MS-SMB2] 2.2.32 bases OutputOffset on InputOffset even with no input.
	le.PutUint32(res[24:28], uint32(off+64)) // InputOffset

	if c.Input != nil {

		c.Input.Encode(res[off:])

		le.PutUint32(res[28:32], uint32(c.Input.Size())) // InputCount

		off += c.Input.Size()
	}

	if c.Output != nil {
		off = Roundup(off+64, 8) - 64
		le.PutUint32(res[32:36], uint32(off+64)) // OutputOffset

		c.Output.Encode(res[off:])

		le.PutUint32(res[36:40], uint32(c.Output.Size())) // OutputCount
	}
}

type IoctlResponseDecoder []byte

func (r IoctlResponseDecoder) IsInvalidHeader() bool {
	if len(r) < 48 {
		return true
	}

	if r.StructureSize() != 49 {
		return true
	}

	return false
}

func (r IoctlResponseDecoder) IsInvalidPayload() bool {
	const fixedEnd = uint64(64 + 48)

	packetEnd := uint64(len(r)) + 64
	inputOffset := uint64(r.InputOffset())
	inputCount := uint64(r.InputCount())
	if inputCount > 0 {
		if inputOffset < fixedEnd || inputOffset > packetEnd || inputCount > packetEnd-inputOffset {
			return true
		}
	}

	outputOffset := uint64(r.OutputOffset())
	outputCount := uint64(r.OutputCount())
	if outputCount == 0 {
		return false
	}
	if outputOffset < fixedEnd || outputOffset > packetEnd || outputCount > packetEnd-outputOffset {
		return true
	}

	// [MS-SMB2] 2.2.32 requires non-empty buffers after the fixed part and
	// requires non-empty output at InputOffset+InputCount rounded to 8 bytes.
	// Widen the uint32 fields before addition and rounding to avoid overflow.
	inputEnd := inputOffset + inputCount
	return outputOffset != (inputEnd+7)&^uint64(7)
}

func (r IoctlResponseDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r IoctlResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r IoctlResponseDecoder) CtlCode() uint32 {
	return le.Uint32(r[4:8])
}

func (r IoctlResponseDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

func (r IoctlResponseDecoder) InputOffset() uint32 {
	return le.Uint32(r[24:28])
}

func (r IoctlResponseDecoder) InputCount() uint32 {
	return le.Uint32(r[28:32])
}

func (r IoctlResponseDecoder) OutputOffset() uint32 {
	return le.Uint32(r[32:36])
}

func (r IoctlResponseDecoder) OutputCount() uint32 {
	return le.Uint32(r[36:40])
}

func (r IoctlResponseDecoder) Flags() uint32 {
	return le.Uint32(r[40:44])
}

func (r IoctlResponseDecoder) Input() []byte {
	n := int(r.InputCount())
	if n == 0 {
		return nil
	}
	off := int(r.InputOffset()) - 64
	return r[off : off+n]
}

func (r IoctlResponseDecoder) Output() []byte {
	n := int(r.OutputCount())
	if n == 0 {
		return nil
	}
	off := int(r.OutputOffset()) - 64
	return r[off : off+n]
}

// ----------------------------------------------------------------------------
// SMB2 QUERY_DIRECTORY Response
//

type QueryDirectoryResponse struct {
	PacketHeader

	Output Encoder
}

func (c *QueryDirectoryResponse) Command() Command {
	return SMB2_QUERY_DIRECTORY
}

func (c *QueryDirectoryResponse) CreditCharge() uint16 {
	return 1
}

func (c *QueryDirectoryResponse) SetCreditCharge(u uint16) {}

func (c *QueryDirectoryResponse) Size() int {
	if c.Output == nil {
		return 64 + 8 + 1
	}
	return 64 + 8 + c.Output.Size()
}

func (c *QueryDirectoryResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 9) // StructureSize

	off := 8

	if c.Output != nil {
		le.PutUint16(res[2:4], uint16(off+64))
		c.Output.Encode(res[8:])
		le.PutUint32(res[4:8], uint32(c.Output.Size()))
	}
}

type QueryDirectoryResponseDecoder []byte

func (r QueryDirectoryResponseDecoder) IsInvalidHeader() bool {
	if len(r) < 8 {
		return true
	}

	if r.StructureSize() != 9 {
		return true
	}

	return false
}

func (r QueryDirectoryResponseDecoder) IsInvalidPayload() bool {
	if r.OutputBufferLength() > 0 && r.OutputBufferOffset() < 64+8 {
		// Non-empty Buffer starts after the SMB2 header and fixed fields
		// ([MS-SMB2] 2.2.34).
		return true
	}

	return uint64(len(r))+64 < uint64(r.OutputBufferOffset())+uint64(r.OutputBufferLength())
}

func (r QueryDirectoryResponseDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r QueryDirectoryResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r QueryDirectoryResponseDecoder) OutputBufferOffset() uint16 {
	return le.Uint16(r[2:4])
}

func (r QueryDirectoryResponseDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r QueryDirectoryResponseDecoder) Output() []byte {
	length := r.OutputBufferLength()
	if length == 0 {
		return nil
	}
	off := uint32(r.OutputBufferOffset()) - 64
	return r[off : off+length]
}

// ----------------------------------------------------------------------------
// SMB2 CHANGE_NOTIFY Response
//

type ChangeNotifyResponse struct {
	PacketHeader

	Output Encoder
}

func (c *ChangeNotifyResponse) Command() Command {
	return SMB2_CHANGE_NOTIFY
}

func (c *ChangeNotifyResponse) CreditCharge() uint16 {
	return 1
}

func (c *ChangeNotifyResponse) SetCreditCharge(u uint16) {}

func (c *ChangeNotifyResponse) Size() int {
	if c.Output == nil {
		return 64 + 8
	}
	return 64 + 8 + c.Output.Size()
}

func (c *ChangeNotifyResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 9) // StructureSize
	if c.Output != nil {
		le.PutUint16(res[2:4], 64+8)
		le.PutUint32(res[4:8], uint32(c.Output.Size()))
		c.Output.Encode(res[8:])
	}
}

type ChangeNotifyResponseDecoder []byte

func (r ChangeNotifyResponseDecoder) IsInvalidHeader() bool {
	return len(r) < 8 || r.StructureSize() != 9
}

func (r ChangeNotifyResponseDecoder) IsInvalidPayload() bool {
	if len(r) < 8 {
		return true
	}

	packetEnd := uint64(len(r)) + 64
	offset := uint64(r.OutputBufferOffset())
	length := uint64(r.OutputBufferLength())
	if length == 0 {
		if offset == 0 {
			return false
		}
		return offset < 64+8 || offset > packetEnd
	}
	// [MS-SMB2] 2.2.36 defines OutputBufferOffset from the SMB2 header;
	// validate it against this command's compound-packet boundary.
	return offset < 64+8 || offset > packetEnd || length > packetEnd-offset
}

func (r ChangeNotifyResponseDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r ChangeNotifyResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r ChangeNotifyResponseDecoder) OutputBufferOffset() uint16 {
	return le.Uint16(r[2:4])
}

func (r ChangeNotifyResponseDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r ChangeNotifyResponseDecoder) Output() []byte {
	length := r.OutputBufferLength()
	if length == 0 {
		return nil
	}
	off := uint32(r.OutputBufferOffset()) - 64
	return r[off : off+length]
}

// ----------------------------------------------------------------------------
// SMB2 QUERY_INFO Response
//

type QueryInfoResponse struct {
	PacketHeader

	Output Encoder
}

func (c *QueryInfoResponse) Command() Command {
	return SMB2_QUERY_INFO
}

func (c *QueryInfoResponse) CreditCharge() uint16 {
	return 1
}

func (c *QueryInfoResponse) SetCreditCharge(u uint16) {}

func (c *QueryInfoResponse) Size() int {
	if c.Output == nil {
		return 64 + 8 + 1
	}
	return 64 + 8 + c.Output.Size()
}

func (c *QueryInfoResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 9) // StructureSize

	off := 8

	if c.Output != nil {
		le.PutUint16(res[2:4], uint16(off+64))
		c.Output.Encode(res[8:])
		le.PutUint32(res[4:8], uint32(c.Output.Size()))
	}
}

type QueryInfoResponseDecoder []byte

func (r QueryInfoResponseDecoder) IsInvalid() bool {
	if len(r) < 8 {
		return true
	}

	if r.StructureSize() != 9 {
		return true
	}

	if r.OutputBufferLength() != 0 && r.OutputBufferOffset() < 64+8 {
		// Non-empty Buffer starts after the SMB2 header and fixed fields
		// ([MS-SMB2] 2.2.38).
		return true
	}

	if uint64(len(r))+64 < uint64(r.OutputBufferOffset())+uint64(r.OutputBufferLength()) {
		return true
	}

	return false
}

func (r QueryInfoResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r QueryInfoResponseDecoder) OutputBufferOffset() uint16 {
	return le.Uint16(r[2:4])
}

func (r QueryInfoResponseDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r QueryInfoResponseDecoder) Output() []byte {
	length := r.OutputBufferLength()
	if length == 0 {
		return nil
	}
	off := uint32(r.OutputBufferOffset()) - 64
	return r[off : off+length]
}

// ----------------------------------------------------------------------------
// SMB2 SET_INFO Response
//

type SetInfoResponse struct {
	PacketHeader
}

func (c *SetInfoResponse) Command() Command {
	return SMB2_SET_INFO
}

func (c *SetInfoResponse) CreditCharge() uint16 {
	return 1
}

func (c *SetInfoResponse) SetCreditCharge(u uint16) {}

func (c *SetInfoResponse) Size() int {
	return 64 + 2
}

func (c *SetInfoResponse) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	res := pkt[64:]
	le.PutUint16(res[:2], 2) // StructureSize
}

type SetInfoResponseDecoder []byte

func (r SetInfoResponseDecoder) IsInvalid() bool {
	if len(r) < 2 {
		return true
	}

	if r.StructureSize() != 2 {
		return true
	}

	return false
}

func (r SetInfoResponseDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}
