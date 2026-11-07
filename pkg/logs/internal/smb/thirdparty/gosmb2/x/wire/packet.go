package wire

// ----------------------------------------------------------------------------
// SMB2 Packet Header
//

type PacketHeader struct {
	ChannelSequence       uint16
	Status                uint32
	CreditRequestResponse uint16
	Flags                 uint32
	NextCommand           uint32
	MessageId             uint64
	AsyncId               uint64
	TreeId                uint32
	SessionId             uint64
}

func (hdr *PacketHeader) SetMessageId(u uint64) {
	hdr.MessageId = u
}

func (hdr *PacketHeader) SetSessionId(u uint64) {
	hdr.SessionId = u
}

func (hdr *PacketHeader) SetTreeId(u uint32) {
	hdr.TreeId = u
}

func (hdr *PacketHeader) SetNextCommand(u uint32) {
	hdr.NextCommand = u
}

func (hdr *PacketHeader) SetCreditRequestResponse(u uint16) {
	hdr.CreditRequestResponse = u
}

func (hdr *PacketHeader) SetCreditRequest(u uint16) {
	hdr.CreditRequestResponse = u
}

func (hdr *PacketHeader) SetCreditResponse(u uint16) {
	hdr.CreditRequestResponse = u
}

func (hdr *PacketHeader) SetFlags(u uint32) {
	hdr.Flags = u
}

// HeaderFlags returns the SMB2 packet-header Flags field. It is separate from
// request-body Flags fields (for example READ and IOCTL flags).
func (hdr *PacketHeader) HeaderFlags() uint32 { return hdr.Flags }

func (hdr *PacketHeader) encodeHeader(command Command, creditCharge uint16, pkt []byte) {
	p := PacketCodec(pkt)

	p.SetProtocolId()
	p.SetStructureSize()
	p.SetCreditCharge(creditCharge)

	switch {
	case hdr.Flags&SMB2_FLAGS_SERVER_TO_REDIR != 0:
		p.SetStatus(hdr.Status)
	case hdr.ChannelSequence != 0:
		p.SetChannelSequence(hdr.ChannelSequence)
	case hdr.Status != 0:
		p.SetStatus(hdr.Status)
	}

	p.SetCommand(command)
	p.SetCreditRequest(hdr.CreditRequestResponse)
	p.SetFlags(hdr.Flags)
	p.SetNextCommand(hdr.NextCommand)
	p.SetMessageId(hdr.MessageId)

	switch {
	case hdr.Flags&SMB2_FLAGS_ASYNC_COMMAND != 0:
		p.SetAsyncId(hdr.AsyncId)
	case hdr.TreeId != 0:
		p.SetTreeId(hdr.TreeId)
	case hdr.AsyncId != 0:
		p.SetAsyncId(hdr.AsyncId)
	}

	p.SetSessionId(hdr.SessionId)
}

// ----------------------------------------------------------------------------
// SMB2 Packet Interface
//

type Packet interface {
	Encoder

	Command() Command
	CreditCharge() uint16
	SetCreditCharge(u uint16)
	SetMessageId(u uint64)
	SetSessionId(u uint64)
	SetTreeId(u uint32)
	SetNextCommand(u uint32)
	SetCreditRequestResponse(u uint16)
	SetCreditRequest(u uint16)
	SetCreditResponse(u uint16)
	SetFlags(u uint32)
	HeaderFlags() uint32
}

// ----------------------------------------------------------------------------
// SMB2 Packet Header
//

type PacketCodec []byte

func (p PacketCodec) IsInvalid() bool {
	if len(p) < 64 {
		return true
	}

	magic := p.ProtocolId()
	if magic[0] != 0xfe {
		return true
	}
	if magic[1] != 'S' {
		return true
	}
	if magic[2] != 'M' {
		return true
	}
	if magic[3] != 'B' {
		return true
	}

	if p.StructureSize() != 64 {
		return true
	}

	next := p.NextCommand()
	if next&7 != 0 {
		return true
	}
	if next != 0 && (next < 64 || uint64(next) > uint64(len(p))) {
		return true
	}

	return false
}

// IsInvalidResponse reports whether the packet header is invalid as a response.
func (p PacketCodec) IsInvalidResponse() bool {
	return p.IsInvalid() || p.Flags()&SMB2_FLAGS_SERVER_TO_REDIR == 0
}

// IsInvalidRequest reports whether the packet header is invalid as a request.
func (p PacketCodec) IsInvalidRequest() bool {
	return p.IsInvalid() || p.Flags()&SMB2_FLAGS_SERVER_TO_REDIR != 0
}

func (p PacketCodec) ProtocolId() []byte {
	return p[:4]
}

func (p PacketCodec) SetProtocolId() {
	copy(p, MAGIC)
}

func (p PacketCodec) StructureSize() uint16 {
	return le.Uint16(p[4:6])
}

func (p PacketCodec) SetStructureSize() {
	le.PutUint16(p[4:6], 64)
}

func (p PacketCodec) CreditCharge() uint16 {
	return le.Uint16(p[6:8])
}

func (p PacketCodec) SetCreditCharge(u uint16) {
	le.PutUint16(p[6:8], u)
}

func (p PacketCodec) Status() uint32 {
	return le.Uint32(p[8:12])
}

func (p PacketCodec) SetStatus(u uint32) {
	le.PutUint32(p[8:12], u)
}

func (p PacketCodec) Command() Command {
	return Command(le.Uint16(p[12:14]))
}

func (p PacketCodec) SetCommand(u Command) {
	le.PutUint16(p[12:14], uint16(u))
}

func (p PacketCodec) CreditRequest() uint16 {
	return le.Uint16(p[14:16])
}

func (p PacketCodec) SetCreditRequest(u uint16) {
	le.PutUint16(p[14:16], u)
}

func (p PacketCodec) CreditResponse() uint16 {
	return le.Uint16(p[14:16])
}

func (p PacketCodec) SetCreditResponse(u uint16) {
	le.PutUint16(p[14:16], u)
}

func (p PacketCodec) Flags() uint32 {
	return le.Uint32(p[16:20])
}

func (p PacketCodec) SetFlags(u uint32) {
	le.PutUint32(p[16:20], u)
}

func (p PacketCodec) NextCommand() uint32 {
	return le.Uint32(p[20:24])
}

func (p PacketCodec) SetNextCommand(u uint32) {
	le.PutUint32(p[20:24], u)
}

func (p PacketCodec) MessageId() uint64 {
	return le.Uint64(p[24:32])
}

func (p PacketCodec) SetMessageId(u uint64) {
	le.PutUint64(p[24:32], u)
}

func (p PacketCodec) AsyncId() uint64 {
	return le.Uint64(p[32:40])
}

func (p PacketCodec) SetAsyncId(u uint64) {
	le.PutUint64(p[32:40], u)
}

func (p PacketCodec) TreeId() uint32 {
	return le.Uint32(p[36:40])
}

func (p PacketCodec) SetTreeId(u uint32) {
	le.PutUint32(p[36:40], u)
}

func (p PacketCodec) SessionId() uint64 {
	return le.Uint64(p[40:48])
}

func (p PacketCodec) SetSessionId(u uint64) {
	le.PutUint64(p[40:48], u)
}

func (p PacketCodec) Signature() []byte {
	return p[48:64]
}

func (p PacketCodec) SetSignature(bs []byte) {
	copy(p[48:64], bs)
}

func (p PacketCodec) Body() []byte {
	return p[64:]
}

// From SMB3

func (p PacketCodec) ChannelSequence() uint16 {
	return le.Uint16(p[8:10])
}

func (p PacketCodec) SetChannelSequence(u uint16) {
	le.PutUint16(p[8:10], u)
}

// ----------------------------------------------------------------------------
// SMB2 TRANSFORM_HEADER
//

// From SMB3

type TransformCodec []byte

func (p TransformCodec) IsInvalid() bool {
	// MS-SMB2 3.2.5.1.1.1 requires ciphertext after the 52-byte header.
	// It may hold a compressed message smaller than an SMB2 header; validate
	// the inner SMB2 structure only after decryption and decompression.
	if len(p) <= 52 {
		return true
	}
	if uint64(52)+uint64(p.OriginalMessageSize()) != uint64(len(p)) {
		return true
	}

	magic := p.ProtocolId()
	if magic[0] != 0xfd || magic[1] != 'S' || magic[2] != 'M' || magic[3] != 'B' {
		return true
	}

	if p.Flags() != Encrypted {
		return true
	}

	return false
}

func (p TransformCodec) ProtocolId() []byte {
	return p[:4]
}

func (p TransformCodec) SetProtocolId() {
	copy(p[:4], MAGIC2)
}

func (p TransformCodec) Signature() []byte {
	return p[4:20]
}

func (p TransformCodec) SetSignature(bs []byte) {
	copy(p[4:20], bs)
}

func (p TransformCodec) Nonce() []byte {
	return p[20:36]
}

func (p TransformCodec) SetNonce(bs []byte) {
	copy(p[20:36], bs)
}

func (p TransformCodec) OriginalMessageSize() uint32 {
	return le.Uint32(p[36:40])
}

func (p TransformCodec) SetOriginalMessageSize(u uint32) {
	le.PutUint32(p[36:40], u)
}

func (p TransformCodec) EncryptionAlgorithm() uint16 {
	return le.Uint16(p[42:44])
}

func (p TransformCodec) SetEncryptionAlgorithm(u uint16) {
	le.PutUint16(p[42:44], u)
}

func (p TransformCodec) SessionId() uint64 {
	return le.Uint64(p[44:52])
}

func (p TransformCodec) SetSessionId(u uint64) {
	le.PutUint64(p[44:52], u)
}

func (p TransformCodec) AssociatedData() []byte {
	return p[20:52]
}

func (p TransformCodec) EncryptedData() []byte {
	return p[52:]
}

// From SMB311

func (p TransformCodec) Flags() uint16 {
	return le.Uint16(p[42:44])
}

func (p TransformCodec) SetFlags(u uint16) {
	le.PutUint16(p[42:44], u)
}

// ----------------------------------------------------------------------------
// SMB2 COMPRESSION_TRANSFORM_HEADER_UNCHAINED
//

type CompressionCodec []byte

func (p CompressionCodec) IsInvalid() bool {
	if len(p) < 16 {
		return true
	}

	magic := p.ProtocolId()
	if magic[0] != 0xfc || magic[1] != 'S' || magic[2] != 'M' || magic[3] != 'B' {
		return true
	}

	if p.Flags() != 0 {
		return true
	}

	algo := p.CompressionAlgorithm()
	if algo > 5 {
		return true
	}

	offset := p.Offset()
	if offset&7 != 0 || uint64(offset) > uint64(len(p)-16) {
		return true
	}

	return false
}

func (p CompressionCodec) ProtocolId() []byte {
	return p[:4]
}

func (p CompressionCodec) SetProtocolId() {
	copy(p[:4], MAGIC3)
}

func (p CompressionCodec) OriginalCompressedSegmentSize() uint32 {
	return le.Uint32(p[4:8])
}

func (p CompressionCodec) SetOriginalCompressedSegmentSize(u uint32) {
	le.PutUint32(p[4:8], u)
}

func (p CompressionCodec) CompressionAlgorithm() uint16 {
	return le.Uint16(p[8:10])
}

func (p CompressionCodec) SetCompressionAlgorithm(u uint16) {
	le.PutUint16(p[8:10], u)
}

func (p CompressionCodec) Flags() uint16 {
	return le.Uint16(p[10:12])
}

func (p CompressionCodec) SetFlags(u uint16) {
	le.PutUint16(p[10:12], u)
}

func (p CompressionCodec) Offset() uint32 {
	return le.Uint32(p[12:16])
}

func (p CompressionCodec) SetOffset(u uint32) {
	le.PutUint32(p[12:16], u)
}

func (p CompressionCodec) CompressedData() []byte {
	offset := int(p.Offset())
	return p[16+offset:]
}
