package protocol

import "encoding/binary"

const (
	AAPL_SERVER_QUERY uint32 = 0x01
)

// Server query request and response bitmap values.
const (
	AAPL_SERVER_CAPS uint64 = 0x01
	AAPL_VOLUME_CAPS uint64 = 0x02
	AAPL_MODEL_INFO  uint64 = 0x04
)

// Client and server capability bitmap values.
const (
	AAPL_SUPPORTS_READ_DIR_ATTR uint64 = 0x01
	AAPL_SUPPORTS_OSX_COPYFILE  uint64 = 0x02
	AAPL_UNIX_BASED             uint64 = 0x04
	AAPL_SUPPORTS_NFS_ACE       uint64 = 0x08
)

// Volume capability bitmap values.
const (
	AAPL_SUPPORT_RESOLVE_ID uint64 = 0x01
	AAPL_CASE_SENSITIVE     uint64 = 0x02
)

// AaplExtensionContext encodes an AAPL SMB2 CREATE context.
type AaplExtensionContext struct {
	Command            uint32
	RequestBitmap      uint64
	ClientCapabilities uint64
}

func (AaplExtensionContext) Size() int { return 48 }

func (c AaplExtensionContext) Encode(p []byte) {
	clear(p[:48])
	binary.LittleEndian.PutUint16(p[4:6], 16)   // NameOffset
	binary.LittleEndian.PutUint16(p[6:8], 4)    // NameLength
	binary.LittleEndian.PutUint16(p[10:12], 24) // DataOffset
	binary.LittleEndian.PutUint32(p[12:16], 24) // DataLength
	copy(p[16:20], "AAPL")
	binary.LittleEndian.PutUint32(p[24:28], c.Command)
	binary.LittleEndian.PutUint64(p[32:40], c.RequestBitmap)
	binary.LittleEndian.PutUint64(p[40:48], c.ClientCapabilities)
}
