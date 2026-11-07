package wire

import (
	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

// ----------------------------------------------------------------------------
// SMB2 NEGOTIATE Request Packet
//

type NegotiateRequest struct {
	PacketHeader

	SecurityMode uint16
	Capabilities uint32
	ClientGuid   uuid.UUID
	Dialects     []Dialect

	Contexts NegotiateContexts
}

func (c *NegotiateRequest) Command() Command {
	return SMB2_NEGOTIATE
}

func (c *NegotiateRequest) CreditCharge() uint16 {
	return 1
}

func (c *NegotiateRequest) SetCreditCharge(u uint16) {}

func (c *NegotiateRequest) Size() int {
	size := 36 + len(c.Dialects)*2
	if len(c.Contexts) > 0 {
		size = Roundup(size, 8) + c.Contexts.Size()
	}

	return 64 + size
}

func (c *NegotiateRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 36) // StructureSize
	le.PutUint16(req[4:6], c.SecurityMode)
	le.PutUint32(req[8:12], c.Capabilities)
	encodeGUID(c.ClientGuid, req[12:28])

	{
		bs := req[36:]
		for i, d := range c.Dialects {
			le.PutUint16(bs[2*i:2*i+2], uint16(d))
		}
		le.PutUint16(req[2:4], uint16(len(c.Dialects)))
	}

	if len(c.Contexts) > 0 {
		off := Roundup(36+len(c.Dialects)*2, 8)

		le.PutUint32(req[28:32], uint32(off+64)) // NegotiateContextOffset

		c.Contexts.Encode(req[off:])
	}

	le.PutUint16(req[32:34], uint16(len(c.Contexts))) // NegotiateContextCount
}

type NegotiateRequestDecoder []byte

func (r NegotiateRequestDecoder) IsInvalid() bool {
	if len(r) < 36 {
		return true
	}

	if r.StructureSize() != 36 {
		return true
	}

	if uint64(len(r)) < 36+2*uint64(r.DialectCount()) {
		return true
	}

	// [MS-SMB2] 2.2.3 places the NegotiateContextList after the Dialects
	// array, aligned to 8 bytes. When SMB 3.1.1 is offered and contexts are
	// present, the context array cannot overlap the fixed structure or the
	// dialects. Reading the raw Dialects array is safe because the length
	// check above already bounds it.
	hasSMB311 := false
	for i := 0; i < int(r.DialectCount()); i++ {
		if Dialect(le.Uint16(r[36+2*i:38+2*i])) == SMB311 {
			hasSMB311 = true
			break
		}
	}

	noff := r.NegotiateContextOffset()
	if hasSMB311 {
		if noff&7 != 0 {
			return true
		}
		if noff != 0 && (noff < 64 || uint64(noff)-64 > uint64(len(r))) {
			return true
		}
	}

	if hasSMB311 && r.NegotiateContextCount() > 0 {
		minimum := (64 + 36 + 2*uint64(r.DialectCount()) + 7) &^ uint64(7)
		if uint64(noff) < minimum {
			return true
		}
		list := NegotiateContextsDecoder(r[int(noff)-64:])
		if list.IsInvalid() || list.Count() != int(r.NegotiateContextCount()) {
			return true
		}
	}

	return false
}

func (r NegotiateRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r NegotiateRequestDecoder) DialectCount() uint16 {
	return le.Uint16(r[2:4])
}

func (r NegotiateRequestDecoder) SecurityMode() uint16 {
	return le.Uint16(r[4:6])
}

func (r NegotiateRequestDecoder) Capabilities() uint32 {
	return le.Uint32(r[8:12])
}

func (r NegotiateRequestDecoder) ClientGuid() uuid.UUID {
	return decodeGUID(r[12:28])
}

func (r NegotiateRequestDecoder) ClientStartTime() []byte {
	return r[28:36]
}

func (r NegotiateRequestDecoder) Dialects() []Dialect {
	// [MS-SMB2] 2.2.3: DialectCount is the number of 16-bit Dialects
	// entries; widen before calculating the variable-length field boundary.
	count := int(r.DialectCount())
	end := 36 + 2*count
	bs := r[36:end]
	us := make([]Dialect, len(bs)/2)
	for i := range us {
		us[i] = Dialect(le.Uint16(bs[2*i : 2*i+2]))
	}
	return us
}

// From SMB311

func (r NegotiateRequestDecoder) NegotiateContextOffset() uint32 {
	return le.Uint32(r[28:32])
}

func (r NegotiateRequestDecoder) NegotiateContextCount() uint16 {
	return le.Uint16(r[32:34])
}

func (r NegotiateRequestDecoder) Contexts() NegotiateContextsDecoder {
	if r.NegotiateContextCount() == 0 {
		return nil
	}
	hasSMB311 := false
	for _, dialect := range r.Dialects() {
		if dialect == SMB311 {
			hasSMB311 = true
			break
		}
	}
	if !hasSMB311 {
		return nil
	}
	off := r.NegotiateContextOffset()
	if off == 0 {
		return nil
	}
	return NegotiateContextsDecoder(r[off-64:])
}

// ----------------------------------------------------------------------------
// SMB2 SESSION_SETUP Request Packet
//

type SessionSetupRequest struct {
	PacketHeader

	Flags             uint8
	SecurityMode      uint8
	Capabilities      uint32
	Channel           uint32
	SecurityBuffer    []byte
	PreviousSessionId uint64
}

func (c *SessionSetupRequest) Command() Command {
	return SMB2_SESSION_SETUP
}

func (c *SessionSetupRequest) CreditCharge() uint16 {
	return 1
}

func (c *SessionSetupRequest) SetCreditCharge(u uint16) {}

func (c *SessionSetupRequest) Size() int {
	if len(c.SecurityBuffer) == 0 {
		return 64 + 24 + 1
	}
	return 64 + 24 + len(c.SecurityBuffer)
}

func (c *SessionSetupRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 25)
	req[2] = c.Flags
	req[3] = c.SecurityMode
	le.PutUint32(req[4:8], c.Capabilities)
	le.PutUint32(req[8:12], c.Channel)
	le.PutUint64(req[16:24], c.PreviousSessionId)

	// SecurityBuffer
	{
		copy(req[24:], c.SecurityBuffer)
		le.PutUint16(req[12:14], 64+24)                         // SecurityBufferOffset
		le.PutUint16(req[14:16], uint16(len(c.SecurityBuffer))) // SecurityBufferLength
	}
}

type SessionSetupRequestDecoder []byte

func (r SessionSetupRequestDecoder) IsInvalid() bool {
	if len(r) < 24 {
		return true
	}

	if r.StructureSize() != 25 {
		return true
	}

	// [MS-SMB2] 2.2.5 defines SecurityBuffer after the 24-byte fixed fields,
	// which follow the 64-byte SMB2 header, so a non-empty buffer must start
	// at offset 88 or later.
	if r.SecurityBufferLength() > 0 && r.SecurityBufferOffset() < 64+24 {
		return true
	}

	if uint64(len(r))+64 < uint64(r.SecurityBufferOffset())+uint64(r.SecurityBufferLength()) {
		return true
	}

	return false
}

func (r SessionSetupRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r SessionSetupRequestDecoder) Flags() uint8 {
	return r[2]
}

func (r SessionSetupRequestDecoder) SecurityMode() uint8 {
	return r[3]
}

func (r SessionSetupRequestDecoder) Capabilities() uint32 {
	return le.Uint32(r[4:8])
}

func (r SessionSetupRequestDecoder) Channel() uint32 {
	return le.Uint32(r[8:12])
}

func (r SessionSetupRequestDecoder) PreviousSessionId() uint64 {
	return le.Uint64(r[16:24])
}

func (r SessionSetupRequestDecoder) SecurityBufferOffset() uint16 {
	return le.Uint16(r[12:14])
}

func (r SessionSetupRequestDecoder) SecurityBufferLength() uint16 {
	return le.Uint16(r[14:16])
}

func (r SessionSetupRequestDecoder) SecurityBuffer() []byte {
	n := int(r.SecurityBufferLength())
	if n == 0 {
		return nil
	}
	off := int(r.SecurityBufferOffset()) - 64
	return r[off : off+n]
}

// ----------------------------------------------------------------------------
// SMB2 LOGOFF Request Packet
//

type LogoffRequest struct {
	PacketHeader
}

func (c *LogoffRequest) Command() Command {
	return SMB2_LOGOFF
}

func (c *LogoffRequest) CreditCharge() uint16 {
	return 1
}

func (c *LogoffRequest) SetCreditCharge(u uint16) {}

func (c *LogoffRequest) Size() int {
	return 64 + 4
}

func (c *LogoffRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 4) // StructureSize
}

type LogoffRequestDecoder []byte

func (r LogoffRequestDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r LogoffRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 ECHO Request Packet
//

type EchoRequest struct {
	PacketHeader
}

func (c *EchoRequest) Command() Command {
	return SMB2_ECHO
}

func (c *EchoRequest) CreditCharge() uint16 {
	return 1
}

func (c *EchoRequest) SetCreditCharge(u uint16) {}

func (c *EchoRequest) Size() int {
	return 64 + 4
}

func (c *EchoRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 4) // StructureSize
}

type EchoRequestDecoder []byte

func (r EchoRequestDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r EchoRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 TREE_CONNECT Request Packet
//

type TreeConnectRequest struct {
	PacketHeader

	Flags uint16
	Path  string
}

func (c *TreeConnectRequest) Command() Command {
	return SMB2_TREE_CONNECT
}

func (c *TreeConnectRequest) CreditCharge() uint16 {
	return 1
}

func (c *TreeConnectRequest) SetCreditCharge(u uint16) {}

func (c *TreeConnectRequest) Size() int {
	if len(c.Path) == 0 {
		return 64 + 8 + 1
	}

	return 64 + 8 + utf16le.EncodedStringLen(c.Path)
}

func (c *TreeConnectRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 9) // StructureSize
	le.PutUint16(req[2:4], c.Flags)

	// Path
	{
		plen := utf16le.EncodeString(req[8:], c.Path)

		le.PutUint16(req[4:6], 8+64)         // PathOffset
		le.PutUint16(req[6:8], uint16(plen)) // PathLength
	}
}

type TreeConnectRequestDecoder []byte

func (r TreeConnectRequestDecoder) IsInvalid() bool {
	if len(r) < 8 {
		return true
	}

	if r.StructureSize() != 9 {
		return true
	}

	plen := uint64(r.PathLength())
	if plen&1 != 0 {
		return true
	}

	poff := uint64(r.PathOffset())
	if plen > 0 && poff < 64+8 {
		return true
	}

	if uint64(len(r))+64 < poff+plen {
		return true
	}

	if plen == 0 {
		return true
	}

	off := poff - 64
	return IsInvalidSharePath(r[off : off+plen])
}

func (r TreeConnectRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r TreeConnectRequestDecoder) Flags() uint16 {
	return le.Uint16(r[2:4])
}

func (r TreeConnectRequestDecoder) PathOffset() uint16 {
	return le.Uint16(r[4:6])
}

func (r TreeConnectRequestDecoder) PathLength() uint16 {
	return le.Uint16(r[6:8])
}

func (r TreeConnectRequestDecoder) Path() string {
	off := int(r.PathOffset()) - 64
	return utf16le.DecodeToString(r[off : off+int(r.PathLength())])
}

// ----------------------------------------------------------------------------
// SMB2 TREE_DISCONNECT Request Packet
//

type TreeDisconnectRequest struct {
	PacketHeader
}

func (c *TreeDisconnectRequest) Command() Command {
	return SMB2_TREE_DISCONNECT
}

func (c *TreeDisconnectRequest) CreditCharge() uint16 {
	return 1
}

func (c *TreeDisconnectRequest) SetCreditCharge(u uint16) {}

func (c *TreeDisconnectRequest) Size() int {
	return 64 + 4
}

func (c *TreeDisconnectRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 4) // StructureSize
}

type TreeDisconnectRequestDecoder []byte

func (r TreeDisconnectRequestDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r TreeDisconnectRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 CREATE Request Packet
//

type CreateRequest struct {
	PacketHeader

	SecurityFlags        uint8
	RequestedOplockLevel uint8
	ImpersonationLevel   uint32
	SmbCreateFlags       uint64
	DesiredAccess        uint32
	FileAttributes       uint32
	ShareAccess          uint32
	CreateDisposition    uint32
	CreateOptions        uint32
	Name                 string

	Contexts CreateContexts
}

func (c *CreateRequest) Command() Command {
	return SMB2_CREATE
}

func (c *CreateRequest) CreditCharge() uint16 {
	return 1
}

func (c *CreateRequest) SetCreditCharge(u uint16) {
}

func (c *CreateRequest) Size() int {
	if len(c.Name) == 0 && len(c.Contexts) == 0 {
		return 64 + 56 + 1
	}

	size := 64 + 56 + utf16le.EncodedStringLen(c.Name)
	if len(c.Contexts) > 0 {
		size = Roundup(size, 8) + c.Contexts.Size()
	}

	return size
}

func (c *CreateRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 57) // StructureSize
	req[2] = c.SecurityFlags
	req[3] = c.RequestedOplockLevel
	le.PutUint32(req[4:8], c.ImpersonationLevel)
	le.PutUint64(req[8:16], c.SmbCreateFlags)
	le.PutUint32(req[24:28], c.DesiredAccess)
	le.PutUint32(req[28:32], c.FileAttributes)
	le.PutUint32(req[32:36], c.ShareAccess)
	le.PutUint32(req[36:40], c.CreateDisposition)
	le.PutUint32(req[40:44], c.CreateOptions)

	// Name
	nlen := utf16le.EncodeString(req[56:], c.Name)

	le.PutUint16(req[44:46], 56+64)
	le.PutUint16(req[46:48], uint16(nlen))

	if len(c.Contexts) > 0 {
		off := Roundup(56+nlen, 8)

		le.PutUint32(req[48:52], uint32(64+off))            // CreateContextsOffset
		le.PutUint32(req[52:56], uint32(c.Contexts.Size())) // CreateContextsLength

		c.Contexts.Encode(req[off:])
	}
}

type CreateRequestDecoder []byte

func (r CreateRequestDecoder) IsInvalid() bool {
	if len(r) < 56 {
		return true
	}

	if r.StructureSize() != 57 {
		return true
	}

	nlen := uint64(r.NameLength())
	if nlen&1 != 0 {
		return true
	}

	noff := uint64(r.NameOffset())
	if noff&7 != 0 {
		return true
	}

	if nlen > 0 && noff < 64+56 {
		return true
	}

	if uint64(len(r))+64 < noff+nlen {
		return true
	}

	coff := uint64(r.CreateContextsOffset())
	if coff&7 != 0 {
		return true
	}

	clen := uint64(r.CreateContextsLength())
	if clen == 0 {
		if coff != 0 {
			return true
		}
	} else if coff < 64+56 {
		return true
	}

	if uint64(len(r))+64 < coff+clen {
		return true
	}

	if clen > 0 {
		if CreateContextsDecoder(r[int(coff)-64 : int(coff)-64+int(clen)]).IsInvalid() {
			return true
		}
		for _, context := range r.Contexts().Contexts() {
			if data := createContextData(context, "QFid"); len(data) != 0 {
				return true
			}
		}
	}

	return r.CreateOptions()&FILE_OPEN_BY_FILE_ID == 0 && nlen > 0 && isInvalidUTF16LE(r[noff-64:noff-64+nlen])
}

func (r CreateRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r CreateRequestDecoder) SecurityFlags() uint8 {
	return r[2]
}

func (r CreateRequestDecoder) RequestedOplockLevel() uint8 {
	return r[3]
}

func (r CreateRequestDecoder) ImpersonationLevel() uint32 {
	return le.Uint32(r[4:8])
}

func (r CreateRequestDecoder) SmbCreateFlags() uint64 {
	return le.Uint64(r[8:16])
}

func (r CreateRequestDecoder) DesiredAccess() uint32 {
	return le.Uint32(r[24:28])
}

func (r CreateRequestDecoder) FileAttributes() uint32 {
	return le.Uint32(r[28:32])
}

func (r CreateRequestDecoder) ShareAccess() uint32 {
	return le.Uint32(r[32:36])
}

func (r CreateRequestDecoder) CreateDisposition() uint32 {
	return le.Uint32(r[36:40])
}

func (r CreateRequestDecoder) CreateOptions() uint32 {
	return le.Uint32(r[40:44])
}

func (r CreateRequestDecoder) NameOffset() uint16 {
	return le.Uint16(r[44:46])
}

func (r CreateRequestDecoder) NameLength() uint16 {
	return le.Uint16(r[46:48])
}

func (r CreateRequestDecoder) Name() string {
	length := int(r.NameLength())
	if length == 0 {
		return ""
	}
	off := int(r.NameOffset()) - 64
	return utf16le.DecodeToString(r[off : off+length])
}

func (r CreateRequestDecoder) CreateContextsOffset() uint32 {
	return le.Uint32(r[48:52])
}

func (r CreateRequestDecoder) CreateContextsLength() uint32 {
	return le.Uint32(r[52:56])
}

func (r CreateRequestDecoder) Contexts() CreateContextsDecoder {
	length := int(r.CreateContextsLength())
	if length == 0 {
		return nil
	}
	off := int(r.CreateContextsOffset()) - 64
	return CreateContextsDecoder(r[off : off+length])
}

// ----------------------------------------------------------------------------
// SMB2 CLOSE Request Packet
//

type CloseRequest struct {
	PacketHeader

	Flags  uint16
	FileId FileId
}

func (c *CloseRequest) Command() Command {
	return SMB2_CLOSE
}

func (c *CloseRequest) CreditCharge() uint16 {
	return 1
}

func (c *CloseRequest) SetCreditCharge(u uint16) {}

func (c *CloseRequest) Size() int {
	return 64 + 24
}

func (c *CloseRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 24) // StructureSize
	le.PutUint16(req[2:4], c.Flags)
	c.FileId.Encode(req[8:24])
}

type CloseRequestDecoder []byte

func (r CloseRequestDecoder) IsInvalid() bool {
	if len(r) < 24 {
		return true
	}

	if r.StructureSize() != 24 {
		return true
	}

	return false
}

func (r CloseRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r CloseRequestDecoder) Flags() uint16 {
	return le.Uint16(r[2:4])
}

func (r CloseRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

// ----------------------------------------------------------------------------
// SMB2 FLUSH Request Packet
//

type FlushRequest struct {
	PacketHeader

	FileId FileId
}

func (c *FlushRequest) Command() Command {
	return SMB2_FLUSH
}

func (c *FlushRequest) CreditCharge() uint16 {
	return 1
}

func (c *FlushRequest) SetCreditCharge(u uint16) {}

func (c *FlushRequest) Size() int {
	return 64 + 24
}

func (c *FlushRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 24) // StructureSize
	c.FileId.Encode(req[8:24])
}

type FlushRequestDecoder []byte

func (r FlushRequestDecoder) IsInvalid() bool {
	if len(r) < 24 {
		return true
	}

	if r.StructureSize() != 24 {
		return true
	}

	return false
}

func (r FlushRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r FlushRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

// ----------------------------------------------------------------------------
// SMB2 READ Request Packet
//

type ReadRequest struct {
	PacketHeader

	creditCharge uint16

	Padding         uint8
	Flags           uint8
	Length          uint32
	Offset          uint64
	FileId          FileId
	MinimumCount    uint32
	Channel         uint32
	RemainingBytes  uint32
	ReadChannelInfo []Encoder
}

func (c *ReadRequest) Command() Command {
	return SMB2_READ
}

func (c *ReadRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *ReadRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *ReadRequest) Size() int {
	if len(c.ReadChannelInfo) == 0 {
		return 64 + 48 + 1
	}

	size := 64 + 48
	for _, r := range c.ReadChannelInfo {
		size += r.Size()
	}
	return size
}

func (c *ReadRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 49)
	req[2] = c.Padding
	req[3] = c.Flags
	le.PutUint32(req[4:8], c.Length)
	le.PutUint64(req[8:16], c.Offset)
	c.FileId.Encode(req[16:32])
	le.PutUint32(req[32:36], c.MinimumCount)
	le.PutUint32(req[36:40], c.Channel)
	le.PutUint32(req[40:44], c.RemainingBytes)

	off := 48

	for i, r := range c.ReadChannelInfo {
		if i == 0 {
			le.PutUint16(req[44:46], uint16(64+off)) // ReadChannelInfoOffset
		}

		r.Encode(req[off:])

		off += r.Size()
	}

	le.PutUint16(req[46:48], uint16(off-48)) // ReadChannelInfoLength
}

type ReadRequestDecoder []byte

func (r ReadRequestDecoder) IsInvalid() bool {
	if len(r) < 48 {
		return true
	}

	if r.StructureSize() != 49 {
		return true
	}

	// [MS-SMB2] 2.2.19 defines ReadChannelInfo after the 48-byte fixed fields,
	// which follow the 64-byte SMB2 header, so a non-empty buffer must start
	// at offset 112 or later.
	if r.ReadChannelInfoLength() > 0 && r.ReadChannelInfoOffset() < 64+48 {
		return true
	}

	if uint64(len(r))+64 < uint64(r.ReadChannelInfoOffset())+uint64(r.ReadChannelInfoLength()) {
		return true
	}

	return false
}

func (r ReadRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r ReadRequestDecoder) Padding() uint8 {
	return r[2]
}

func (r ReadRequestDecoder) Flags() uint8 {
	return r[3]
}

func (r ReadRequestDecoder) Length() uint32 {
	return le.Uint32(r[4:8])
}

func (r ReadRequestDecoder) Offset() uint64 {
	return le.Uint64(r[8:16])
}

func (r ReadRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[16:32])
}

func (r ReadRequestDecoder) MinimumCount() uint32 {
	return le.Uint32(r[32:36])
}

func (r ReadRequestDecoder) Channel() uint32 {
	return le.Uint32(r[36:40])
}

func (r ReadRequestDecoder) RemainingBytes() uint32 {
	return le.Uint32(r[40:44])
}

func (r ReadRequestDecoder) ReadChannelInfoOffset() uint16 {
	return le.Uint16(r[44:46])
}

func (r ReadRequestDecoder) ReadChannelInfoLength() uint16 {
	return le.Uint16(r[46:48])
}

func (r ReadRequestDecoder) ReadChannelInfo() []byte {
	length := int(r.ReadChannelInfoLength())
	if length == 0 {
		return nil
	}
	off := int(r.ReadChannelInfoOffset()) - 64
	return r[off : off+length]
}

// ----------------------------------------------------------------------------
// SMB2 WRITE Request Packet
//

type WriteRequest struct {
	PacketHeader

	creditCharge uint16

	FileId           FileId
	Flags            uint32
	Channel          uint32
	RemainingBytes   uint32
	Offset           uint64
	WriteChannelInfo []Encoder
	Data             []byte
}

func (c *WriteRequest) Command() Command {
	return SMB2_WRITE
}

func (c *WriteRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *WriteRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *WriteRequest) Size() int {
	if len(c.Data) == 0 && len(c.WriteChannelInfo) == 0 {
		return 64 + 48 + 1
	}

	off := 64 + 48

	for _, w := range c.WriteChannelInfo {
		off += w.Size()
	}

	off += len(c.Data)

	return off
}

func (c *WriteRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 49) // StructureSize
	le.PutUint64(req[8:16], c.Offset)
	c.FileId.Encode(req[16:32])
	le.PutUint32(req[32:36], c.Channel)
	le.PutUint32(req[36:40], c.RemainingBytes)
	le.PutUint32(req[44:48], c.Flags)

	off := 48

	for i, w := range c.WriteChannelInfo {
		if i == 0 {
			le.PutUint16(req[40:42], uint16(64+off)) // WriteChannelInfoOffset
		}

		w.Encode(req[off:])

		off += w.Size()
	}

	le.PutUint16(req[42:44], uint16(off-48)) // WriteChannelInfoLength

	le.PutUint16(req[2:4], uint16(64+off)) // DataOffset

	copy(req[off:], c.Data)

	le.PutUint32(req[4:8], uint32(len(c.Data))) // Length
}

type WriteRequestDecoder []byte

func (r WriteRequestDecoder) IsInvalidHeader() bool {
	if len(r) < 48 {
		return true
	}

	if r.StructureSize() != 49 {
		return true
	}

	return false
}

func (r WriteRequestDecoder) IsInvalidPayload() bool {
	// [MS-SMB2] 2.2.21 defines WriteChannelInfo and Data after the 48-byte
	// fixed fields, which follow the 64-byte SMB2 header, so a non-empty
	// region must start at offset 112 or later.
	if r.WriteChannelInfoLength() > 0 && r.WriteChannelInfoOffset() < 64+48 {
		return true
	}

	if r.Length() > 0 && r.DataOffset() < 64+48 {
		return true
	}

	if uint64(len(r))+64 < uint64(r.WriteChannelInfoOffset())+uint64(r.WriteChannelInfoLength()) {
		return true
	}

	if uint64(len(r))+64 < uint64(r.DataOffset())+uint64(r.Length()) {
		return true
	}

	return false
}

func (r WriteRequestDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r WriteRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r WriteRequestDecoder) DataOffset() uint16 {
	return le.Uint16(r[2:4])
}

func (r WriteRequestDecoder) Length() uint32 {
	return le.Uint32(r[4:8])
}

func (r WriteRequestDecoder) Data() []byte {
	length := int(r.Length())
	if length == 0 {
		return nil
	}
	off := int(r.DataOffset()) - 64
	return r[off : off+length]
}

func (r WriteRequestDecoder) Offset() uint64 {
	return le.Uint64(r[8:16])
}

func (r WriteRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[16:32])
}

func (r WriteRequestDecoder) Channel() uint32 {
	return le.Uint32(r[32:36])
}

func (r WriteRequestDecoder) RemainingBytes() uint32 {
	return le.Uint32(r[36:40])
}

func (r WriteRequestDecoder) WriteChannelInfoOffset() uint16 {
	return le.Uint16(r[40:42])
}

func (r WriteRequestDecoder) WriteChannelInfoLength() uint16 {
	return le.Uint16(r[42:44])
}

func (r WriteRequestDecoder) WriteChannelInfo() []byte {
	length := int(r.WriteChannelInfoLength())
	if length == 0 {
		return nil
	}
	off := int(r.WriteChannelInfoOffset()) - 64
	return r[off : off+length]
}

func (r WriteRequestDecoder) Flags() uint32 {
	return le.Uint32(r[44:48])
}

// ----------------------------------------------------------------------------
// SMB2 OPLOCK_BREAK Acknowledgement
//

// ----------------------------------------------------------------------------
// SMB2 LOCK Request Packet
//

type LockRequest struct {
	PacketHeader

	FileId FileId
	// LockSequence packs LockSequenceNumber into the low four bits and
	// LockSequenceIndex into the high 28 bits. The public client leaves both
	// values zero because it does not use resilient, durable, or multichannel
	// opens ([MS-SMB2] 3.2.4.19).
	LockSequence uint32
	Locks        []LockElement
}

func (c *LockRequest) Command() Command {
	return SMB2_LOCK
}

func (c *LockRequest) CreditCharge() uint16 {
	return 1
}

func (c *LockRequest) SetCreditCharge(u uint16) {}

func (c *LockRequest) Size() int {
	return 64 + 24 + len(c.Locks)*24
}

func (c *LockRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	// [MS-SMB2] 2.2.26 requires StructureSize to remain 48 for every
	// LockCount. The sequence field remains zero for this client.
	le.PutUint16(req[:2], 48)
	le.PutUint16(req[2:4], uint16(len(c.Locks)))
	le.PutUint32(req[4:8], c.LockSequence)
	c.FileId.Encode(req[8:24])
	for i, lock := range c.Locks {
		off := 24 + i*24
		lock.Encode(req[off : off+24])
	}
}

type LockElement struct {
	Offset uint64
	Length uint64
	Flags  uint32
}

func (c LockElement) Size() int {
	return 24
}

func (c LockElement) Encode(dst []byte) {
	le.PutUint64(dst[:8], c.Offset)
	le.PutUint64(dst[8:16], c.Length)
	le.PutUint32(dst[16:20], c.Flags)
}

type LockRequestDecoder []byte

func (r LockRequestDecoder) IsInvalid() bool {
	if len(r) < 24 || r.StructureSize() != 48 || r.LockCount() == 0 {
		return true
	}
	count := uint64(r.LockCount())
	if count > (uint64(^uint(0)>>1)-24)/24 || uint64(len(r)) < 24+count*24 {
		return true
	}
	locks := r.Locks()
	for i := uint16(0); i < r.LockCount(); i++ {
		lock := LockElementDecoder(locks[:24])
		if lock.IsInvalid() {
			return true
		}
		locks = locks[24:]
	}
	return false
}

func (r LockRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r LockRequestDecoder) LockCount() uint16 {
	return le.Uint16(r[2:4])
}

func (r LockRequestDecoder) LockSequence() uint32 {
	return le.Uint32(r[4:8])
}

func (r LockRequestDecoder) LockSequenceNumber() uint32 {
	return r.LockSequence() & 0x0f
}

func (r LockRequestDecoder) LockSequenceIndex() uint32 {
	return r.LockSequence() >> 4
}

func (r LockRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

func (r LockRequestDecoder) Locks() []byte {
	return r[24 : 24+int(r.LockCount())*24]
}

type LockElementDecoder []byte

func (r LockElementDecoder) IsInvalid() bool {
	if len(r) < 24 {
		return true
	}
	switch r.Flags() {
	case SMB2_LOCKFLAG_SHARED_LOCK,
		SMB2_LOCKFLAG_EXCLUSIVE_LOCK,
		SMB2_LOCKFLAG_SHARED_LOCK | SMB2_LOCKFLAG_FAIL_IMMEDIATELY,
		SMB2_LOCKFLAG_EXCLUSIVE_LOCK | SMB2_LOCKFLAG_FAIL_IMMEDIATELY,
		SMB2_LOCKFLAG_UNLOCK:
		return false
	default:
		return true
	}
}

func (r LockElementDecoder) Offset() uint64 {
	return le.Uint64(r[:8])
}

func (r LockElementDecoder) Length() uint64 {
	return le.Uint64(r[8:16])
}

func (r LockElementDecoder) Flags() uint32 {
	return le.Uint32(r[16:20])
}

// ----------------------------------------------------------------------------
// SMB2 ECHO Request Packet
//

// ----------------------------------------------------------------------------
// SMB2 CANCEL Request Packet
//

type CancelRequest struct {
	PacketHeader
}

func (c *CancelRequest) Command() Command {
	return SMB2_CANCEL
}

func (c *CancelRequest) CreditCharge() uint16 {
	return 0
}

func (c *CancelRequest) SetCreditCharge(u uint16) {}

func (c *CancelRequest) Size() int {
	return 64 + 4
}

func (c *CancelRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 4) // StructureSize
}

type CancelRequestDecoder []byte

func (r CancelRequestDecoder) IsInvalid() bool {
	if len(r) < 4 {
		return true
	}

	if r.StructureSize() != 4 {
		return true
	}

	return false
}

func (r CancelRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

// ----------------------------------------------------------------------------
// SMB2 IOCTL Request Packet
//

type IoctlRequest struct {
	PacketHeader

	creditCharge uint16

	CtlCode           uint32
	FileId            FileId
	OutputOffset      uint32
	OutputCount       uint32
	MaxInputResponse  uint32
	MaxOutputResponse uint32
	Flags             uint32
	Input             Encoder
}

func (c *IoctlRequest) Command() Command {
	return SMB2_IOCTL
}

func (c *IoctlRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *IoctlRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *IoctlRequest) Size() int {
	if c.Input == nil {
		return 64 + 56 + 1
	}

	return 64 + 56 + c.Input.Size()
}

func (c *IoctlRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 57) // StructureSize
	le.PutUint32(req[4:8], c.CtlCode)
	c.FileId.Encode(req[8:24])
	le.PutUint32(req[32:36], c.MaxInputResponse)
	le.PutUint32(req[36:40], c.OutputOffset)
	le.PutUint32(req[40:44], c.OutputCount)
	le.PutUint32(req[44:48], c.MaxOutputResponse)
	le.PutUint32(req[48:52], c.Flags)

	off := 56

	if c.Input != nil {
		le.PutUint32(req[24:28], uint32(off+64)) // InputOffset

		c.Input.Encode(req[off:])

		le.PutUint32(req[28:32], uint32(c.Input.Size())) // InputCount
	}
}

type IoctlRequestDecoder []byte

func (r IoctlRequestDecoder) IsInvalidHeader() bool {
	if len(r) < 56 {
		return true
	}

	if r.StructureSize() != 57 {
		return true
	}

	return false
}

func (r IoctlRequestDecoder) IsInvalidPayload() bool {
	// [MS-SMB2] 2.2.31 defines Input after the 56-byte fixed fields, which
	// follow the 64-byte SMB2 header, so a non-empty buffer must start at
	// offset 120 or later.
	if r.InputCount() > 0 && r.InputOffset() < 64+56 {
		return true
	}

	return uint64(len(r))+64 < uint64(r.InputOffset())+uint64(r.InputCount())
}

func (r IoctlRequestDecoder) IsInvalid() bool {
	return r.IsInvalidHeader() || r.IsInvalidPayload()
}

func (r IoctlRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r IoctlRequestDecoder) CtlCode() uint32 {
	return le.Uint32(r[4:8])
}

func (r IoctlRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

func (r IoctlRequestDecoder) InputOffset() uint32 {
	return le.Uint32(r[24:28])
}

func (r IoctlRequestDecoder) InputCount() uint32 {
	return le.Uint32(r[28:32])
}

func (r IoctlRequestDecoder) Input() []byte {
	length := int(r.InputCount())
	if length == 0 {
		return nil
	}
	off := int(r.InputOffset()) - 64
	return r[off : off+length]
}

func (r IoctlRequestDecoder) MaxInputResponse() uint32 {
	return le.Uint32(r[32:36])
}

func (r IoctlRequestDecoder) OutputOffset() uint32 {
	return le.Uint32(r[36:40])
}

func (r IoctlRequestDecoder) OutputCount() uint32 {
	return le.Uint32(r[40:44])
}

func (r IoctlRequestDecoder) MaxOutputResponse() uint32 {
	return le.Uint32(r[44:48])
}

func (r IoctlRequestDecoder) Flags() uint32 {
	return le.Uint32(r[48:52])
}

// ----------------------------------------------------------------------------
// SMB2 QUERY_DIRECTORY Request Packet
//

type QueryDirectoryRequest struct {
	PacketHeader

	creditCharge uint16

	FileInfoClass      uint8
	Flags              uint8
	FileIndex          uint32
	FileId             FileId
	OutputBufferLength uint32
	FileName           string
}

func (c *QueryDirectoryRequest) Command() Command {
	return SMB2_QUERY_DIRECTORY
}

func (c *QueryDirectoryRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *QueryDirectoryRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *QueryDirectoryRequest) Size() int {
	if len(c.FileName) == 0 {
		return 64 + 32 + 1
	}

	return 64 + 32 + utf16le.EncodedStringLen(c.FileName)
}

func (c *QueryDirectoryRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 33) // StructureSize
	req[2] = c.FileInfoClass
	req[3] = c.Flags
	le.PutUint32(req[4:8], c.FileIndex)
	c.FileId.Encode(req[8:24])
	le.PutUint32(req[28:32], c.OutputBufferLength)

	if len(c.FileName) > 0 {
		off := 32

		le.PutUint16(req[24:26], uint16(off+64)) // FileNameOffset

		flen := utf16le.EncodeString(req[off:], c.FileName)

		le.PutUint16(req[26:28], uint16(flen)) // FileNameLength
	}
}

type QueryDirectoryRequestDecoder []byte

func (r QueryDirectoryRequestDecoder) IsInvalid() bool {
	if len(r) < 32 {
		return true
	}

	if r.StructureSize() != 33 {
		return true
	}

	nlen := uint64(r.FileNameLength())
	if nlen&1 != 0 {
		return true
	}

	noff := uint64(r.FileNameOffset())
	if nlen == 0 {
		if noff != 0 {
			return true
		}
	} else if noff < 64+32 {
		return true
	}

	if uint64(len(r))+64 < noff+nlen {
		return true
	}

	return nlen > 0 && isInvalidUTF16LE(r[noff-64:noff-64+nlen])
}

func (r QueryDirectoryRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r QueryDirectoryRequestDecoder) FileInfoClass() uint8 {
	return r[2]
}

func (r QueryDirectoryRequestDecoder) Flags() uint8 {
	return r[3]
}

func (r QueryDirectoryRequestDecoder) FileIndex() uint32 {
	return le.Uint32(r[4:8])
}

func (r QueryDirectoryRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

func (r QueryDirectoryRequestDecoder) FileNameOffset() uint16 {
	return le.Uint16(r[24:26])
}

func (r QueryDirectoryRequestDecoder) FileNameLength() uint16 {
	return le.Uint16(r[26:28])
}

func (r QueryDirectoryRequestDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[28:32])
}

func (r QueryDirectoryRequestDecoder) FileName() string {
	n := int(r.FileNameLength())
	if n == 0 {
		return ""
	}
	off := int(r.FileNameOffset()) - 64
	return utf16le.DecodeToString(r[off : off+n])
}

// ----------------------------------------------------------------------------
// SMB2 CHANGE_NOTIFY Request Packet
//

type ChangeNotifyRequest struct {
	PacketHeader

	Flags              uint16
	OutputBufferLength uint32
	FileId             FileId
	CompletionFilter   uint32
}

func (c *ChangeNotifyRequest) Command() Command {
	return SMB2_CHANGE_NOTIFY
}

func (c *ChangeNotifyRequest) CreditCharge() uint16 {
	return 1
}

func (c *ChangeNotifyRequest) SetCreditCharge(u uint16) {}

func (c *ChangeNotifyRequest) Size() int {
	return 64 + 32
}

func (c *ChangeNotifyRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 32) // StructureSize
	le.PutUint16(req[2:4], c.Flags)
	le.PutUint32(req[4:8], c.OutputBufferLength)
	c.FileId.Encode(req[8:24])
	le.PutUint32(req[24:28], c.CompletionFilter)
}

type ChangeNotifyRequestDecoder []byte

func (r ChangeNotifyRequestDecoder) IsInvalid() bool {
	if len(r) < 32 || r.StructureSize() != 32 {
		return true
	}
	return r.Flags()&^uint16(SMB2_WATCH_TREE) != 0
}

func (r ChangeNotifyRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r ChangeNotifyRequestDecoder) Flags() uint16 {
	return le.Uint16(r[2:4])
}

func (r ChangeNotifyRequestDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r ChangeNotifyRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[8:24])
}

func (r ChangeNotifyRequestDecoder) CompletionFilter() uint32 {
	return le.Uint32(r[24:28])
}

// ----------------------------------------------------------------------------
// SMB2 QUERY_INFO Request Packet
//

type QueryInfoRequest struct {
	PacketHeader

	creditCharge uint16

	InfoType              uint8
	FileInfoClass         uint8
	OutputBufferLength    uint32
	AdditionalInformation uint32
	Flags                 uint32
	FileId                FileId
	Input                 Encoder
}

func (c *QueryInfoRequest) Command() Command {
	return SMB2_QUERY_INFO
}

func (c *QueryInfoRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *QueryInfoRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *QueryInfoRequest) Size() int {
	if c.Input == nil {
		return 64 + 40 + 1
	}

	return 64 + 40 + c.Input.Size()
}

func (c *QueryInfoRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 41) // StructureSize
	req[2] = c.InfoType
	req[3] = c.FileInfoClass
	le.PutUint32(req[4:8], c.OutputBufferLength)
	le.PutUint32(req[16:20], c.AdditionalInformation)
	le.PutUint32(req[20:24], c.Flags)
	c.FileId.Encode(req[24:40])

	off := 40

	if c.Input != nil {
		le.PutUint16(req[8:10], uint16(off+64)) // InputBufferOffset

		c.Input.Encode(req[off:])

		le.PutUint32(req[12:16], uint32(c.Input.Size())) // InputBufferLength
	}
}

type QueryInfoRequestDecoder []byte

func (r QueryInfoRequestDecoder) IsInvalid() bool {
	if len(r) < 40 {
		return true
	}

	if r.StructureSize() != 41 {
		return true
	}

	// [MS-SMB2] 2.2.37 defines InputBuffer after the 40-byte fixed fields,
	// which follow the 64-byte SMB2 header, so a non-empty buffer must start
	// at offset 104 or later.
	if r.InputBufferLength() > 0 && r.InputBufferOffset() < 64+40 {
		return true
	}

	if uint64(len(r))+64 < uint64(r.InputBufferOffset())+uint64(r.InputBufferLength()) {
		return true
	}

	return false
}

func (r QueryInfoRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r QueryInfoRequestDecoder) InfoType() uint8 {
	return r[2]
}

func (r QueryInfoRequestDecoder) FileInfoClass() uint8 {
	return r[3]
}

func (r QueryInfoRequestDecoder) OutputBufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r QueryInfoRequestDecoder) InputBufferOffset() uint16 {
	return le.Uint16(r[8:10])
}

func (r QueryInfoRequestDecoder) InputBufferLength() uint32 {
	return le.Uint32(r[12:16])
}

func (r QueryInfoRequestDecoder) Input() []byte {
	length := int(r.InputBufferLength())
	if length == 0 {
		return nil
	}
	off := int(r.InputBufferOffset()) - 64
	return r[off : off+length]
}

func (r QueryInfoRequestDecoder) AdditionalInformation() uint32 {
	return le.Uint32(r[16:20])
}

func (r QueryInfoRequestDecoder) Flags() uint32 {
	return le.Uint32(r[20:24])
}

func (r QueryInfoRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[24:40])
}

// ----------------------------------------------------------------------------
// SMB2 SET_INFO Request Packet
//

type SetInfoRequest struct {
	PacketHeader

	creditCharge uint16

	InfoType              uint8
	FileInfoClass         uint8
	AdditionalInformation uint32
	FileId                FileId
	Input                 Encoder
}

func (c *SetInfoRequest) Command() Command {
	return SMB2_SET_INFO
}

func (c *SetInfoRequest) CreditCharge() uint16 {
	if c.creditCharge == 0 {
		return 1
	}
	return c.creditCharge
}

func (c *SetInfoRequest) SetCreditCharge(u uint16) {
	c.creditCharge = u
}

func (c *SetInfoRequest) Size() int {
	if c.Input == nil {
		return 64 + 32 + 1
	}

	return 64 + 32 + c.Input.Size()
}

func (c *SetInfoRequest) Encode(pkt []byte) {
	c.encodeHeader(c.Command(), c.CreditCharge(), pkt)

	req := pkt[64:]
	le.PutUint16(req[:2], 33) // StructureSize
	req[2] = c.InfoType
	req[3] = c.FileInfoClass
	le.PutUint32(req[12:16], c.AdditionalInformation)
	c.FileId.Encode(req[16:32])

	off := 32

	if c.Input != nil {
		le.PutUint16(req[8:10], uint16(off+64)) // BufferOffset

		c.Input.Encode(req[off:])

		le.PutUint32(req[4:8], uint32(c.Input.Size())) // BufferLength
	}
}

type SetInfoRequestDecoder []byte

func (r SetInfoRequestDecoder) IsInvalid() bool {
	if len(r) < 32 {
		return true
	}

	if r.StructureSize() != 33 {
		return true
	}

	// [MS-SMB2] 2.2.39 defines Buffer after the 32-byte fixed fields, which
	// follow the 64-byte SMB2 header, so a non-empty buffer must start at
	// offset 96 or later.
	if r.BufferLength() > 0 && r.BufferOffset() < 64+32 {
		return true
	}

	if uint64(len(r))+64 < uint64(r.BufferOffset())+uint64(r.BufferLength()) {
		return true
	}

	return false
}

func (r SetInfoRequestDecoder) StructureSize() uint16 {
	return le.Uint16(r[:2])
}

func (r SetInfoRequestDecoder) InfoType() uint8 {
	return r[2]
}

func (r SetInfoRequestDecoder) FileInfoClass() uint8 {
	return r[3]
}

func (r SetInfoRequestDecoder) BufferLength() uint32 {
	return le.Uint32(r[4:8])
}

func (r SetInfoRequestDecoder) BufferOffset() uint16 {
	return le.Uint16(r[8:10])
}

func (r SetInfoRequestDecoder) Input() []byte {
	length := int(r.BufferLength())
	if length == 0 {
		return nil
	}
	off := int(r.BufferOffset()) - 64
	return r[off : off+length]
}

func (r SetInfoRequestDecoder) AdditionalInformation() uint32 {
	return le.Uint32(r[12:16])
}

func (r SetInfoRequestDecoder) FileId() FileIdDecoder {
	return FileIdDecoder(r[16:32])
}
