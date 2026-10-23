package wire

// ----------------------------------------------------------------------------
// SMB2 FILEID
//

type FileId struct {
	Persistent [8]byte
	Volatile   [8]byte
}

var RelatedFileId = FileId{
	Persistent: [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	Volatile:   [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
}

func (fd FileId) IsZero() bool {
	for _, b := range fd.Persistent[:] {
		if b != 0 {
			return false
		}
	}
	for _, b := range fd.Volatile[:] {
		if b != 0 {
			return false
		}
	}
	return true
}

func (fd FileId) IsRelated() bool {
	return fd.Persistent == RelatedFileId.Persistent && fd.Volatile == RelatedFileId.Volatile
}

func (fd FileId) Size() int {
	return 16
}

func (fd FileId) Encode(p []byte) {
	copy(p[:8], fd.Persistent[:])
	copy(p[8:16], fd.Volatile[:])
}

type FileIdDecoder []byte

func (fd FileIdDecoder) IsInvalid() bool {
	return len(fd) < 16
}

func (fd FileIdDecoder) Persistent() []byte {
	return fd[:8]
}

func (fd FileIdDecoder) Volatile() []byte {
	return fd[8:16]
}

func (fd FileIdDecoder) Decode() FileId {
	var ret FileId
	copy(ret.Persistent[:], fd[:8])
	copy(ret.Volatile[:], fd[8:16])
	return ret
}

// ----------------------------------------------------------------------------
// SMB2 NEGOTIATE Contexts
//

// From SMB311

type HashContext struct {
	HashAlgorithms []uint16
	HashSalt       []byte
}

func (c *HashContext) Size() int {
	return 8 + 4 + len(c.HashAlgorithms)*2 + len(c.HashSalt)
}

func (c *HashContext) Encode(p []byte) {
	le.PutUint16(p[:2], SMB2_PREAUTH_INTEGRITY_CAPABILITIES)                // ContextType
	le.PutUint16(p[2:4], uint16(4+len(c.HashAlgorithms)*2+len(c.HashSalt))) // DataLength

	{
		d := NegotiateContextDecoder(p).Data()

		// HashAlgorithms
		{
			bs := d[4:]
			for i, alg := range c.HashAlgorithms {
				le.PutUint16(bs[2*i:2*i+2], alg)
			}
			le.PutUint16(d[:2], uint16(len(c.HashAlgorithms)))
		}

		// HashSalt
		{
			off := 4 + len(c.HashAlgorithms)*2
			copy(d[off:], c.HashSalt)
			le.PutUint16(d[2:4], uint16(len(c.HashSalt)))
		}
	}
}

type CipherContext struct {
	Ciphers []Cipher
}

func (c *CipherContext) Size() int {
	return 8 + 2 + len(c.Ciphers)*2
}

type CompressionContext struct {
	CompressionAlgorithms []uint16
	Flags                 uint32
}

func (c *CompressionContext) Size() int {
	return 8 + 8 + len(c.CompressionAlgorithms)*2
}

func (c *CompressionContext) Encode(p []byte) {
	le.PutUint16(p[:2], SMB2_COMPRESSION_CAPABILITIES)
	le.PutUint16(p[2:4], uint16(8+len(c.CompressionAlgorithms)*2))

	d := NegotiateContextDecoder(p).Data()
	le.PutUint16(d[:2], uint16(len(c.CompressionAlgorithms)))
	le.PutUint32(d[4:8], c.Flags)
	for i, algorithm := range c.CompressionAlgorithms {
		le.PutUint16(d[8+2*i:10+2*i], algorithm)
	}
}

func (c *CipherContext) Encode(p []byte) {
	le.PutUint16(p[:2], SMB2_ENCRYPTION_CAPABILITIES) // ContextType
	le.PutUint16(p[2:4], uint16(2+len(c.Ciphers)*2))  // DataLength

	{
		d := NegotiateContextDecoder(p).Data()

		{ // Ciphers
			bs := d[2:]
			for i, c := range c.Ciphers {
				le.PutUint16(bs[2*i:2*i+2], uint16(c))
			}
			le.PutUint16(d[:2], uint16(len(c.Ciphers))) // CipherCount
		}
	}
}

// From SMB311

type NegotiateContextDecoder []byte

func (ctx NegotiateContextDecoder) IsInvalid() bool {
	if len(ctx) < 8 {
		return true
	}

	if len(ctx) < 8+int(ctx.DataLength()) {
		return true
	}

	return false
}

func (ctx NegotiateContextDecoder) ContextType() uint16 {
	return le.Uint16(ctx[:2])
}

func (ctx NegotiateContextDecoder) DataLength() uint16 {
	return le.Uint16(ctx[2:4])
}

func (ctx NegotiateContextDecoder) Data() []byte {
	// [MS-SMB2] 2.2.3.1: DataLength is a 2-byte length of the Data field that
	// follows the 8-byte context header. Widen to int before adding so a
	// maximum DataLength cannot wrap and truncate the slice.
	end := 8 + int(ctx.DataLength())
	return ctx[8:end]
}

func (ctx NegotiateContextDecoder) Next() int {
	return Roundup(8+int(ctx.DataLength()), 8)
}

// NegotiateContexts is the 8-byte-aligned list of negotiate contexts that
// follows a NEGOTIATE request or response ([MS-SMB2] 2.2.3.1). The list starts
// at an 8-byte-aligned offset, so padding before the first context is not part
// of Size.
type NegotiateContexts []Encoder

func (c NegotiateContexts) Size() int {
	size := 0
	for _, ctx := range c {
		size = Roundup(size, 8)
		size += ctx.Size()
	}
	return size
}

func (c NegotiateContexts) Encode(p []byte) {
	off := 0
	for _, ctx := range c {
		off = Roundup(off, 8)
		ctx.Encode(p[off:])
		off += ctx.Size()
	}
}

type NegotiateContextsDecoder []byte

// IsInvalid walks the context list. Each context is self-describing through
// DataLength and 8-byte-aligned; the final context ends exactly at the buffer
// end because the encoder adds no trailing padding.
func (c NegotiateContextsDecoder) IsInvalid() bool {
	for off := 0; off < len(c); {
		if len(c)-off < 8 {
			return true
		}
		ctx := NegotiateContextDecoder(c[off:])
		if ctx.IsInvalid() {
			return true
		}
		end := off + 8 + int(ctx.DataLength())
		next := Roundup(end, 8)
		if next > len(c) {
			return end != len(c)
		}
		off = next
	}
	return false
}

func (c NegotiateContextsDecoder) Contexts() []NegotiateContextDecoder {
	var contexts []NegotiateContextDecoder
	for off := 0; off < len(c); {
		ctx := NegotiateContextDecoder(c[off:])
		contexts = append(contexts, ctx)
		off += ctx.Next()
	}
	return contexts
}

// Count returns the number of contexts in the list. It must be called after
// IsInvalid returns false.
func (c NegotiateContextsDecoder) Count() int {
	count := 0
	for off := 0; off < len(c); {
		ctx := NegotiateContextDecoder(c[off:])
		end := off + 8 + int(ctx.DataLength())
		next := Roundup(end, 8)
		if next > len(c) {
			return count + 1
		}
		count++
		off = next
	}
	return count
}

// CreateContexts is the 8-byte-aligned list of SMB2_CREATE_CONTEXT structures
// carried by a CREATE request or response ([MS-SMB2] 2.2.13.2). Each context
// starts with a 4-byte Next field that CreateContexts maintains: the aligned
// distance to the following context, or zero for the last one. The list starts
// at an 8-byte-aligned offset, so padding before the first context is not part
// of Size.
type CreateContexts []Encoder

func (c CreateContexts) Size() int {
	size := 0
	for _, ctx := range c {
		size = Roundup(size, 8)
		size += ctx.Size()
	}
	return size
}

func (c CreateContexts) Encode(p []byte) {
	off := 0
	for i, ctx := range c {
		off = Roundup(off, 8)
		ctx.Encode(p[off:])

		// The Next field lives in the first 4 bytes of the context. Small or
		// nonconforming contexts are left as-is rather than panicking.
		if ctx.Size() >= 4 {
			next := uint32(0)
			if i+1 < len(c) {
				next = uint32(Roundup(ctx.Size(), 8))
			}
			le.PutUint32(p[off:off+4], next)
		}

		off += ctx.Size()
	}
}

type CreateContextsDecoder []byte

// IsInvalid walks the context list using the Next fields. The last context is
// the one whose Next is zero; the list must not end exactly after a non-final
// context.
func (c CreateContextsDecoder) IsInvalid() bool {
	for off := 0; ; {
		if off == len(c) {
			return true
		}
		if len(c)-off < 16 {
			return true
		}
		next := int(le.Uint32(c[off : off+4]))
		var entryLen int
		if next == 0 {
			entryLen = len(c) - off
		} else {
			if next < 16 || next&7 != 0 || off+next > len(c) {
				return true
			}
			entryLen = next
		}
		nameOffset := uint64(le.Uint16(c[off+4 : off+6]))
		nameLength := uint64(le.Uint16(c[off+6 : off+8]))
		dataOffset := uint64(le.Uint16(c[off+10 : off+12]))
		dataLength := uint64(le.Uint32(c[off+12 : off+16]))
		if nameLength > 0 {
			if nameOffset < 16 || nameOffset+nameLength > uint64(entryLen) {
				return true
			}
		}
		if dataLength > 0 {
			if dataOffset < 16 || dataOffset+dataLength > uint64(entryLen) {
				return true
			}
		}
		if nameLength > 0 && dataLength > 0 {
			if nameOffset < dataOffset+dataLength && dataOffset < nameOffset+nameLength {
				return true
			}
		}
		if next == 0 {
			return false
		}
		off += next
	}
}

// Contexts returns each SMB2_CREATE_CONTEXT as a slice. It must be called
// after IsInvalid returns false.
func (c CreateContextsDecoder) Contexts() [][]byte {
	var contexts [][]byte
	for off := 0; ; {
		next := int(le.Uint32(c[off : off+4]))
		if next == 0 {
			contexts = append(contexts, c[off:])
			break
		}
		contexts = append(contexts, c[off:off+next])
		off += next
	}
	return contexts
}

// QueryOnDiskIDRequest requests the file and volume identifiers in a CREATE
// response ([MS-SMB2] 2.2.13.2.9). Its context data is empty.
type QueryOnDiskIDRequest struct{}

func (QueryOnDiskIDRequest) Size() int { return 24 }

func (QueryOnDiskIDRequest) Encode(p []byte) {
	clear(p[:24])
	le.PutUint16(p[4:6], 16)
	le.PutUint16(p[6:8], 4)
	copy(p[16:20], "QFid")
}

// QueryOnDiskIDResponseDecoder decodes the 32-byte QFid response data
// ([MS-SMB2] 2.2.14.2.9). Reserved bytes are ignored on receipt.
type QueryOnDiskIDResponseDecoder []byte

func (c QueryOnDiskIDResponseDecoder) IsInvalid() bool    { return len(c) != 32 }
func (c QueryOnDiskIDResponseDecoder) DiskFileId() uint64 { return le.Uint64(c[:8]) }
func (c QueryOnDiskIDResponseDecoder) VolumeId() uint64   { return le.Uint64(c[8:16]) }

// createContextData operates on an entry from a validated context list.
// A non-nil empty slice distinguishes an empty payload from an absent name.
func createContextData(c []byte, name string) []byte {
	nlen := int(le.Uint16(c[6:8]))
	if nlen != len(name) {
		return nil
	}
	noff := int(le.Uint16(c[4:6]))
	if string(c[noff:noff+nlen]) != name {
		return nil
	}
	dlen := int(le.Uint32(c[12:16]))
	if dlen == 0 {
		return c[:0]
	}
	doff := int(le.Uint16(c[10:12]))
	return c[doff : doff+dlen]
}

// From SMB311

type HashContextDataDecoder []byte

func (h HashContextDataDecoder) IsInvalid() bool {
	if len(h) < 4 {
		return true
	}

	if len(h) < 4+int(h.HashAlgorithmCount())*2+int(h.SaltLength()) {
		return true
	}

	return false
}

func (h HashContextDataDecoder) HashAlgorithmCount() uint16 {
	return le.Uint16(h[:2])
}

func (h HashContextDataDecoder) SaltLength() uint16 {
	return le.Uint16(h[2:4])
}

func (h HashContextDataDecoder) HashAlgorithms() []uint16 {
	bs := h[4:]
	algs := make([]uint16, h.HashAlgorithmCount())
	for i := range algs {
		algs[i] = le.Uint16(bs[2*i : 2*i+2])
	}
	return algs
}

func (h HashContextDataDecoder) Salt() []byte {
	// [MS-SMB2] 2.2.3.1.1: HashAlgorithms contains HashAlgorithmCount
	// 16-bit IDs before the variable-length Salt field.
	off := 4 + 2*int(h.HashAlgorithmCount())
	saltLength := int(h.SaltLength())
	return h[off : off+saltLength]
}

type CipherContextDataDecoder []byte

func (c CipherContextDataDecoder) IsInvalid() bool {
	if len(c) < 2 {
		return true
	}

	if len(c) < 2+int(c.CipherCount())*2 {
		return true
	}

	return false
}

func (c CipherContextDataDecoder) CipherCount() uint16 {
	return le.Uint16(c[:2])
}

func (c CipherContextDataDecoder) Ciphers() []Cipher {
	bs := c[2:]
	cs := make([]Cipher, c.CipherCount())
	for i := range cs {
		cs[i] = Cipher(le.Uint16(bs[2*i : 2*i+2]))
	}
	return cs
}

type CompressionContextDataDecoder []byte

func (c CompressionContextDataDecoder) IsInvalid() bool {
	if len(c) < 8 {
		return true
	}
	return uint64(len(c)) < 8+2*uint64(c.CompressionAlgorithmCount())
}

func (c CompressionContextDataDecoder) CompressionAlgorithmCount() uint16 {
	return le.Uint16(c[:2])
}

func (c CompressionContextDataDecoder) Flags() uint32 {
	return le.Uint32(c[4:8])
}

func (c CompressionContextDataDecoder) CompressionAlgorithms() []uint16 {
	algorithms := make([]uint16, c.CompressionAlgorithmCount())
	for i := range algorithms {
		algorithms[i] = le.Uint16(c[8+2*i : 10+2*i])
	}
	return algorithms
}

type QueryQuotaInfo struct {
	ReturnSingle bool
	RestartScan  bool
	Sids         []Sid
}

func (q *QueryQuotaInfo) Size() int {
	if len(q.Sids) == 0 {
		return 16
	}
	if len(q.Sids) == 1 {
		return 16 + q.Sids[0].Size()
	}
	l := 16
	for _, sid := range q.Sids {
		l += 8 + sid.Size()
	}
	return l
}

func (q *QueryQuotaInfo) Encode(p []byte) {
	if q.ReturnSingle {
		p[0] = 1
	}
	if q.RestartScan {
		p[1] = 1
	}
	if len(q.Sids) > 0 {
		if len(q.Sids) == 1 {
			sid := q.Sids[0]
			sid.Encode(p[16:])
			le.PutUint32(p[8:12], uint32(sid.Size()))
			le.PutUint32(p[12:16], 0)
		} else {
			le.PutUint32(p[4:8], uint32(q.Size()-16))
			off := 16
			for i, sid := range q.Sids {
				size := sid.Size()
				sid.Encode(p[off+8:])
				le.PutUint32(p[off:off+4], 0)
				if i+1 < len(q.Sids) {
					le.PutUint32(p[off:off+4], uint32(8+size))
				}
				le.PutUint32(p[off+4:off+8], uint32(size))
				off += 8 + size
			}
		}
	}
}

// TransportContext offers transport security in SMB 3.1.1 negotiation.
type TransportContext struct {
	Flags uint32
}

func (c *TransportContext) Size() int { return 12 }

func (c *TransportContext) Encode(p []byte) {
	le.PutUint16(p[:2], SMB2_TRANSPORT_CAPABILITIES)
	le.PutUint16(p[2:4], 4)
	clear(p[4:8])
	le.PutUint32(p[8:12], c.Flags)
}

type TransportContextDataDecoder []byte

func (d TransportContextDataDecoder) IsInvalid() bool { return len(d) < 4 }
func (d TransportContextDataDecoder) Flags() uint32   { return le.Uint32(d[:4]) }
