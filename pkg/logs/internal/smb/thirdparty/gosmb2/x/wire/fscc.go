// ref: MS-FSCC

package wire

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

const (
	IO_REPARSE_TAG_RESERVED_ZERO   = 0x00000000
	IO_REPARSE_TAG_RESERVED_ONE    = 0x00000001
	IO_REPARSE_TAG_MOUNT_POINT     = 0xA0000003
	IO_REPARSE_TAG_HSM             = 0xC0000004
	IO_REPARSE_TAG_HSM2            = 0x80000006
	IO_REPARSE_TAG_DRIVER_EXTENDER = 0x80000005
	IO_REPARSE_TAG_SIS             = 0x80000007
	IO_REPARSE_TAG_DFS             = 0x8000000A
	IO_REPARSE_TAG_DFSR            = 0x80000012
	IO_REPARSE_TAG_FILTER_MANAGER  = 0x8000000B
	IO_REPARSE_TAG_SYMLINK         = 0xA000000C
	IO_REPARSE_TAG_DEDUP           = 0x80000013
	IO_REPARSE_TAG_AF_UNIX         = 0x80000023
)

const (
	FSCTL_DFS_GET_REFERRALS            = 0x00060194
	FSCTL_PIPE_PEEK                    = 0x0011400C
	FSCTL_PIPE_WAIT                    = 0x00110018
	FSCTL_PIPE_TRANSCEIVE              = 0x0011C017
	FSCTL_SRV_COPYCHUNK                = 0x001440F2
	FSCTL_SRV_ENUMERATE_SNAPSHOTS      = 0x00144064
	FSCTL_SRV_REQUEST_RESUME_KEY       = 0x00140078
	FSCTL_SRV_READ_HASH                = 0x001441bb
	FSCTL_SRV_COPYCHUNK_WRITE          = 0x001480F2
	FSCTL_LMR_REQUEST_RESILIENCY       = 0x001401D4
	FSCTL_QUERY_NETWORK_INTERFACE_INFO = 0x001401FC
	FSCTL_GET_REPARSE_POINT            = 0x000900A8
	FSCTL_SET_REPARSE_POINT            = 0x000900A4
	FSCTL_DFS_GET_REFERRALS_EX         = 0x000601B0
	FSCTL_FILE_LEVEL_TRIM              = 0x00098208
	FSCTL_VALIDATE_NEGOTIATE_INFO      = 0x00140204
)

// ----------------------------------------------------------------------------
// [MS-FSCC] 2.1.5 Pathname Component Validation Helpers
// ----------------------------------------------------------------------------

// IsDotDirectoryName reports whether b is a dot directory name ("." or "..")
// ([MS-FSCC] 2.1.5.1).
func IsDotDirectoryName(b []byte) bool {
	if len(b) == 2 {
		return b[0] == '.' && b[1] == 0
	}
	if len(b) == 4 {
		return b[0] == '.' && b[1] == 0 && b[2] == '.' && b[3] == 0
	}
	return false
}

// IsInvalidDotDirectoryName reports whether b is not a valid dot directory name
// ([MS-FSCC] 2.1.5.1).
func IsInvalidDotDirectoryName(b []byte) bool {
	return !IsDotDirectoryName(b)
}

// IsInvalidFilename reports whether b is an invalid filename ([MS-FSCC] 2.1.5.2).
func IsInvalidFilename(b []byte) bool {
	if isInvalidUTF16LE(b) {
		return true
	}
	n := len(b) / 2
	if n < 1 || n > 255 {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		if b[i+1] == 0 {
			ch := b[i]
			if ch <= 0x1F {
				return true
			}
			switch ch {
			case '"', '\\', '/', ':', '|', '<', '>', '*', '?':
				return true
			}
		}
	}
	return false
}

// IsInvalidShortName validates a UTF-16 short name returned by Windows.
// Unlike the ASCII-only definition in [MS-FSCC] 2.1.5.2.1, Windows can
// generate short names containing extended OEM characters (including DBCS).
// See RtlGenerate8dot3Name's AllowExtendedCharacters parameter:
// https://learn.microsoft.com/windows-hardware/drivers/ddi/ntifs/nf-ntifs-rtlgenerate8dot3name
func IsInvalidShortName(b []byte) bool {
	if isInvalidUTF16LE(b) {
		return true
	}
	n := len(b) / 2
	if n < 1 || n > 12 {
		return true
	}
	dotIndex := -1
	for i := 0; i < len(b); i += 2 {
		ch := le.Uint16(b[i : i+2])
		if ch <= 0x1F || ch == ' ' || (ch >= 0xD800 && ch <= 0xDFFF) {
			return true
		}
		switch ch {
		case '"', '\\', '/', ':', '|', '<', '>', '*', '?':
			return true
		case '.':
			if dotIndex != -1 {
				return true
			}
			dotIndex = i / 2
		}
	}
	if dotIndex == -1 {
		return n > 8
	}
	if dotIndex < 1 || dotIndex > 8 {
		return true
	}
	extLen := n - dotIndex - 1
	return extLen < 1 || extLen > 3
}

// IsInvalidStreamName reports whether b is an invalid streamname component
// ([MS-FSCC] 2.1.5.3).
func IsInvalidStreamName(b []byte) bool {
	if isInvalidUTF16LE(b) {
		return true
	}
	n := len(b) / 2
	if n > 255 {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		if b[i+1] == 0 {
			ch := b[i]
			if ch == 0 || ch == '\\' || ch == '/' || ch == ':' {
				return true
			}
		}
	}
	return false
}

// IsInvalidStreamType reports whether b is an invalid streamtype component
// ([MS-FSCC] 2.1.5.4).
func IsInvalidStreamType(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		if b[i+1] == 0 {
			ch := b[i]
			if ch == 0 || ch == '\\' || ch == '/' || ch == ':' {
				return true
			}
		}
	}
	return false
}

// IsInvalidDirectoryEntryName reports whether b is an invalid directory entry
// name ([MS-FSCC] 2.1.5.1, 2.1.5.2).
func IsInvalidDirectoryEntryName(b []byte) bool {
	if IsDotDirectoryName(b) {
		return false
	}
	return IsInvalidFilename(b)
}

// IsInvalidPathnameComponent reports whether b is an invalid pathname component
// ([MS-FSCC] 2.1.5). A component that is a dot directory name is invalid:
// [MS-FSCC] 2.1.5.1 forbids sending "." or ".." except where explicitly
// permitted, and pathname components have no such permit (unlike the
// enumeration FileName fields in [MS-FSCC] 2.4.10 and 2.4.22).
func IsInvalidPathnameComponent(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 {
		return true
	}
	if IsDotDirectoryName(b) {
		return true
	}
	c1, c2 := -1, -1
	for i := 0; i < len(b); i += 2 {
		if b[i] == ':' && b[i+1] == 0 {
			if c1 == -1 {
				c1 = i
			} else if c2 == -1 {
				c2 = i
			} else {
				return true
			}
		}
	}
	if c1 == -1 {
		return IsInvalidFilename(b)
	}
	if c2 == -1 {
		fn := b[:c1]
		sn := b[c1+2:]
		if IsInvalidFilename(fn) {
			return true
		}
		if len(sn) == 0 || IsInvalidStreamName(sn) {
			return true
		}
		return false
	}
	fn := b[:c1]
	sn := b[c1+2 : c2]
	st := b[c2+2:]
	if IsInvalidFilename(fn) {
		return true
	}
	if len(sn) > 0 && IsInvalidStreamName(sn) {
		return true
	}
	if IsInvalidStreamType(st) {
		return true
	}
	return false
}

// IsInvalidPathname reports whether b is an invalid pathname ([MS-FSCC] 2.1.5).
func IsInvalidPathname(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 {
		return true
	}
	if len(b)/2 > 32760 {
		return true
	}
	if len(b) >= 2 && b[0] == '\\' && b[1] == 0 {
		b = b[2:]
		if len(b) == 0 {
			return false
		}
	}
	start := 0
	for i := 0; i < len(b); i += 2 {
		if b[i] == '\\' && b[i+1] == 0 {
			comp := b[start:i]
			// A dot directory name is not a valid intermediate component;
			// IsInvalidDirectoryEntryName permits it for enumeration names
			// ([MS-FSCC] 2.4.10, 2.4.22), so it is rejected separately
			// ([MS-FSCC] 2.1.5.1).
			if IsDotDirectoryName(comp) || IsInvalidDirectoryEntryName(comp) {
				return true
			}
			start = i + 2
		}
	}
	return IsInvalidPathnameComponent(b[start:])
}

// IsInvalidRelativePathname reports whether b is an invalid relative pathname
// ([MS-FSCC] 2.1.5, [MS-SMB2] 3.2.5.16).
func IsInvalidRelativePathname(b []byte) bool {
	if len(b) >= 2 && ((b[0] == '\\' && b[1] == 0) || (b[0] == '/' && b[1] == 0)) {
		return true
	}
	return IsInvalidPathname(b)
}

type SymbolicLinkReparseDataBuffer struct {
	Flags          uint32
	SubstituteName string
	PrintName      string
}

func (c *SymbolicLinkReparseDataBuffer) Size() int {
	return 20 + utf16le.EncodedStringLen(c.SubstituteName) + utf16le.EncodedStringLen(c.PrintName)
}

func (c *SymbolicLinkReparseDataBuffer) Encode(p []byte) {
	slen := utf16le.EncodeString(p[20:], c.SubstituteName)
	plen := utf16le.EncodeString(p[20+slen:], c.PrintName)

	le.PutUint32(p[:4], IO_REPARSE_TAG_SYMLINK)
	le.PutUint16(p[4:6], uint16(c.Size()-8)) // ReparseDataLength excludes compound padding.
	le.PutUint16(p[8:10], 0)                 // SubstituteNameOffset
	le.PutUint16(p[10:12], uint16(slen))     // SubstituteNameLength
	le.PutUint16(p[14:16], uint16(plen))     // PrintNameLength
	le.PutUint16(p[12:14], uint16(slen))     // PrintNameOffset
	le.PutUint32(p[16:20], c.Flags)
}

type SymbolicLinkReparseDataBufferDecoder []byte

func (c SymbolicLinkReparseDataBufferDecoder) IsInvalid() bool {
	if len(c) < 20 {
		return true
	}

	if c.ReparseTag() != IO_REPARSE_TAG_SYMLINK {
		return true
	}

	rlen := int(c.ReparseDataLength())
	soff := int(c.SubstituteNameOffset())
	slen := int(c.SubstituteNameLength())
	poff := int(c.PrintNameOffset())
	plen := int(c.PrintNameLength())

	// These fields are byte lengths or offsets for UTF-16LE Unicode strings;
	// string lengths must therefore be even ([MS-FSCC] 2.1.2.4;
	// [MS-DTYP] 1.1).
	if (soff&1 | poff&1 | slen&1 | plen&1) != 0 {
		return true
	}

	if len(c) < 8+rlen {
		return true
	}

	if rlen < 12+soff+slen || rlen < 12+poff+plen {
		return true
	}

	pathBuffer := c.PathBuffer()
	substituteName := pathBuffer[soff : soff+slen]
	if isInvalidUTF16LE(substituteName) || isInvalidSubstituteName(substituteName, c.Flags()) {
		return true
	}
	if plen > 0 && isInvalidUTF16LE(pathBuffer[poff:poff+plen]) {
		return true
	}

	return false
}

// isInvalidUTF16LE reports malformed UTF-16LE code-unit sequences. The
// protocol decoders must reject malformed input instead of allowing the
// replacement character to hide an invalid surrogate.
func isInvalidUTF16LE(b []byte) bool {
	if len(b)&1 != 0 {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		u := le.Uint16(b[i:])
		if u >= 0xd800 && u <= 0xdbff {
			if i+2 >= len(b) {
				return true
			}
			u2 := le.Uint16(b[i+2:])
			if u2 < 0xdc00 || u2 > 0xdfff {
				return true
			}
			i += 2
		} else if u >= 0xdc00 && u <= 0xdfff {
			return true
		}
	}
	return false
}

func (c SymbolicLinkReparseDataBufferDecoder) ReparseTag() uint32 {
	return le.Uint32(c[:4])
}

func (c SymbolicLinkReparseDataBufferDecoder) ReparseDataLength() uint16 {
	return le.Uint16(c[4:6])
}

func (c SymbolicLinkReparseDataBufferDecoder) SubstituteNameOffset() uint16 {
	return le.Uint16(c[8:10])
}

func (c SymbolicLinkReparseDataBufferDecoder) SubstituteNameLength() uint16 {
	return le.Uint16(c[10:12])
}

func (c SymbolicLinkReparseDataBufferDecoder) PrintNameOffset() uint16 {
	return le.Uint16(c[12:14])
}

func (c SymbolicLinkReparseDataBufferDecoder) PrintNameLength() uint16 {
	return le.Uint16(c[14:16])
}

func (c SymbolicLinkReparseDataBufferDecoder) Flags() uint32 {
	return le.Uint32(c[16:20])
}

func (c SymbolicLinkReparseDataBufferDecoder) PathBuffer() []byte {
	return c[20:]
}

func (c SymbolicLinkReparseDataBufferDecoder) SubstituteName() string {
	buf := c.PathBuffer()
	off := int(c.SubstituteNameOffset())
	length := int(c.SubstituteNameLength())
	return normalizeSymlinkTarget(utf16le.DecodeToString(buf[off : off+length]))
}

func (c SymbolicLinkReparseDataBufferDecoder) PrintName() string {
	off := int(c.PrintNameOffset())
	length := int(c.PrintNameLength())
	buf := c.PathBuffer()
	return utf16le.DecodeToString(buf[off : off+length])
}

// SrvRequestResumeKeyResponse is the response payload of
// FSCTL_SRV_REQUEST_RESUME_KEY.
type SrvRequestResumeKeyResponse struct {
	ResumeKey [24]byte
	Context   []byte
}

func (c *SrvRequestResumeKeyResponse) Size() int {
	return 28 + len(c.Context)
}

func (c *SrvRequestResumeKeyResponse) Encode(p []byte) {
	copy(p[:24], c.ResumeKey[:])
	le.PutUint32(p[24:28], uint32(len(c.Context)))
	copy(p[28:], c.Context)
}

type SrvRequestResumeKeyResponseDecoder []byte

func (c SrvRequestResumeKeyResponseDecoder) IsInvalid() bool {
	if len(c) < 28 {
		return true
	}
	return uint64(len(c)) < 28+uint64(c.ContextLength())
}

func (c SrvRequestResumeKeyResponseDecoder) ResumeKey() []byte {
	return c[:24]
}

func (c SrvRequestResumeKeyResponseDecoder) ContextLength() uint32 {
	return le.Uint32(c[24:28])
}

func (c SrvRequestResumeKeyResponseDecoder) Context() []byte {
	return c[28 : 28+c.ContextLength()]
}

type SrvCopychunkCopy struct {
	SourceKey [24]byte
	Chunks    []SrvCopychunk
}

func (c *SrvCopychunkCopy) Size() int {
	return 32 + len(c.Chunks)*24
}

func (c *SrvCopychunkCopy) Encode(p []byte) {
	copy(p[:24], c.SourceKey[:])
	le.PutUint32(p[24:28], uint32(len(c.Chunks)))
	off := 32
	for i, chunk := range c.Chunks {
		chunk.Encode(p[off+i*24 : off+i*24+24])
	}
}

type SrvCopychunk struct {
	SourceOffset int64
	TargetOffset int64
	Length       uint32
}

func (c *SrvCopychunk) Size() int {
	return 24
}

func (c *SrvCopychunk) Encode(p []byte) {
	le.PutUint64(p[:8], uint64(c.SourceOffset))
	le.PutUint64(p[8:16], uint64(c.TargetOffset))
	le.PutUint32(p[16:20], c.Length)
}

// SrvCopychunkResponse is the response payload of FSCTL_SRV_COPYCHUNK.
type SrvCopychunkResponse struct {
	ChunksWritten      uint32
	ChunksBytesWritten uint32
	TotalBytesWritten  uint32
}

func (c *SrvCopychunkResponse) Size() int {
	return 12
}

func (c *SrvCopychunkResponse) Encode(p []byte) {
	le.PutUint32(p[:4], c.ChunksWritten)
	le.PutUint32(p[4:8], c.ChunksBytesWritten)
	le.PutUint32(p[8:12], c.TotalBytesWritten)
}

type SrvCopychunkResponseDecoder []byte

func (c SrvCopychunkResponseDecoder) IsInvalid() bool {
	return len(c) < 12
}

func (c SrvCopychunkResponseDecoder) ChunksWritten() uint32 {
	return le.Uint32(c[:4])
}

func (c SrvCopychunkResponseDecoder) ChunksBytesWritten() uint32 {
	return le.Uint32(c[4:8])
}

func (c SrvCopychunkResponseDecoder) TotalBytesWritten() uint32 {
	return le.Uint32(c[8:12])
}

const (
	FILE_ATTRIBUTE_ARCHIVE             = 0x20
	FILE_ATTRIBUTE_COMPRESSED          = 0x800
	FILE_ATTRIBUTE_DIRECTORY           = 0x10
	FILE_ATTRIBUTE_ENCRYPTED           = 0x4000
	FILE_ATTRIBUTE_HIDDEN              = 0x2
	FILE_ATTRIBUTE_NORMAL              = 0x80
	FILE_ATTRIBUTE_NOT_CONTENT_INDEXED = 0x2000
	FILE_ATTRIBUTE_OFFLINE             = 0x1000
	FILE_ATTRIBUTE_READONLY            = 0x1
	FILE_ATTRIBUTE_REPARSE_POINT       = 0x400
	FILE_ATTRIBUTE_SPARSE_FILE         = 0x200
	FILE_ATTRIBUTE_SYSTEM              = 0x4
	FILE_ATTRIBUTE_TEMPORARY           = 0x100
	FILE_ATTRIBUTE_INTEGRITY_STREAM    = 0x8000
	FILE_ATTRIBUTE_NO_SCRUB_DATA       = 0x20000
)

const (
	FileDirectoryInformation           = 1 + iota // 1
	FileFullDirectoryInformation                  // 2
	FileBothDirectoryInformation                  // 3
	FileBasicInformation                          // 4
	FileStandardInformation                       // 5
	FileInternalInformation                       // 6
	FileEaInformation                             // 7
	FileAccessInformation                         // 8
	FileNameInformation                           // 9
	FileRenameInformation                         // 10
	FileLinkInformation                           // 11
	FileNamesInformation                          // 12
	FileDispositionInformation                    // 13
	FilePositionInformation                       // 14
	FileFullEaInformation                         // 15
	FileModeInformation                           // 16
	FileAlignmentInformation                      // 17
	FileAllInformation                            // 18
	FileAllocationInformation                     // 19
	FileEndOfFileInformation                      // 20
	FileAlternateNameInformation                  // 21
	FileStreamInformation                         // 22
	FilePipeInformation                           // 23
	FilePipeLocalInformation                      // 24
	FilePipeRemoteInformation                     // 25
	FileMailslotQueryInformation                  // 26
	FileMailslotSetInformation                    // 27
	FileCompressionInformation                    // 28
	FileObjectIdInformation                       // 29
	_                                             // 30
	FileMoveClusterInformation                    // 31
	FileQuotaInformation                          // 32
	FileReparsePointInformation                   // 33
	FileNetworkOpenInformation                    // 34
	FileAttributeTagInformation                   // 35
	FileTrackingInformation                       // 36
	FileIdBothDirectoryInformation                // 37
	FileIdFullDirectoryInformation                // 38
	FileValidDataLengthInformation                // 39
	FileShortNameInformation                      // 40
	_                                             // 41
	_                                             // 42
	_                                             // 43
	FileSfioReserveInformation                    // 44
	FileSfioVolumeInformation                     // 45
	FileHardLinkInformation                       // 46
	_                                             // 47
	FileNormalizedNameInformation                 // 48
	_                                             // 49
	FileIdGlobalTxDirectoryInformation            // 50
	_                                             // 51
	_                                             // 52
	_                                             // 53
	FileStandardLinkInformation                   // 54
)

const (
	FileFsVolumeInformation = 1 + iota
	FileFsLabelInformation
	FileFsSizeInformation
	FileFsDeviceInformation
	FileFsAttributeInformation
	FileFsControlInformation
	FileFsFullSizeInformation
	FileFsObjectIdInformation
	FileFsDriverPathInformation
	FileFsVolumeFlagsInformation
	FileFsSectorSizeInformation
)

// FileNotifyInformationDecoder decodes one FILE_NOTIFY_INFORMATION record.
// The record and its successor are bounded by the complete CHANGE_NOTIFY
// output buffer ([MS-FSCC] 2.7.1).
type FileNotifyInformationDecoder []byte

func (c FileNotifyInformationDecoder) IsInvalid() bool {
	if len(c) < 12 {
		return true
	}

	nameLength := uint64(c.FileNameLength())
	if nameLength == 0 || nameLength&1 != 0 || nameLength > uint64(len(c)-12) {
		return true
	}
	recordLength := uint64(12) + nameLength
	if recordLength > uint64(^uint(0)>>1) {
		return true
	}
	if IsInvalidRelativePathname(c[12 : 12+nameLength]) {
		return true
	}
	// A FILE_NOTIFY_INFORMATION FileName that contains a double quote makes
	// the response invalid ([MS-SMB2] 3.2.5.16). MS-FSCC streamname and
	// streamtype components permit characters that a filename does not, so the
	// declared name must be inspected explicitly. Only the declared name is
	// scanned; the padding before the next 4-byte boundary is not part of it.
	for i := 0; i < int(nameLength); i += 2 {
		if c[12+i] == '"' && c[12+i+1] == 0 {
			return true
		}
	}
	paddedLength := (recordLength + 3) &^ 3
	next := uint64(c.NextEntryOffset())

	if c.Action() < FILE_ACTION_ADDED || c.Action() > FILE_ACTION_TUNNELLED_ID_COLLISION {
		return true
	}
	if next == 0 {
		return uint64(len(c)) != recordLength && uint64(len(c)) != paddedLength
	}
	if next&3 != 0 || next < paddedLength || next > uint64(len(c)) || next == uint64(len(c)) {
		return true
	}
	return uint64(len(c))-next < 12
}

func (c FileNotifyInformationDecoder) NextEntryOffset() uint32 {
	return le.Uint32(c[:4])
}

func (c FileNotifyInformationDecoder) Action() uint32 {
	return le.Uint32(c[4:8])
}

func (c FileNotifyInformationDecoder) FileNameLength() uint32 {
	return le.Uint32(c[8:12])
}

func (c FileNotifyInformationDecoder) FileNameBytes() []byte {
	return c[12 : 12+int(c.FileNameLength())]
}

func (c FileNotifyInformationDecoder) FileName() string {
	return utf16le.DecodeToString(c.FileNameBytes())
}

type FileDirectoryInformationDecoder []byte

func (c FileDirectoryInformationDecoder) IsInvalid() bool {
	if len(c) < 64 {
		return true
	}
	// FILE_DIRECTORY_INFORMATION timestamps must be nonnegative
	// ([MS-FSCC] 2.4.10).
	for _, timestamp := range []FiletimeDecoder{
		c.CreationTime(),
		c.LastAccessTime(),
		c.LastWriteTime(),
		c.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}
	// EndOfFile is signed but must be nonnegative ([MS-FSCC] 2.4.10).
	if c.EndOfFile() < 0 {
		return true
	}
	if c.AllocationSize() < 0 {
		return true
	}
	nameLength := uint64(c.FileNameLength())
	entrySize := 64 + nameLength
	if uint64(len(c)) < entrySize {
		return true
	}
	if IsInvalidDirectoryEntryName(c[64 : 64+nameLength]) {
		return true
	}
	next := uint64(c.NextEntryOffset())
	if next == 0 {
		return false
	}
	if next < entrySize || next >= uint64(len(c)) {
		return true
	}
	if Roundup(int(next), 8) != int(next) {
		return true
	}
	return uint64(len(c))-next < 64
}

func (c FileDirectoryInformationDecoder) NextEntryOffset() uint32 {
	return le.Uint32(c[:4])
}

func (c FileDirectoryInformationDecoder) FileIndex() uint32 {
	return le.Uint32(c[4:8])
}

func (c FileDirectoryInformationDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(c[8:16])
}

func (c FileDirectoryInformationDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(c[16:24])
}

func (c FileDirectoryInformationDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(c[24:32])
}

func (c FileDirectoryInformationDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(c[32:40])
}

func (c FileDirectoryInformationDecoder) EndOfFile() int64 {
	return int64(le.Uint64(c[40:48]))
}

func (c FileDirectoryInformationDecoder) AllocationSize() int64 {
	return int64(le.Uint64(c[48:56]))
}

func (c FileDirectoryInformationDecoder) FileAttributes() uint32 {
	return le.Uint32(c[56:60])
}

func (c FileDirectoryInformationDecoder) FileNameLength() uint32 {
	return le.Uint32(c[60:64])
}

func (c FileDirectoryInformationDecoder) FileName() string {
	return utf16le.DecodeToString(c[64 : 64+c.FileNameLength()])
}

// FileIdBothDirectoryInformationDecoder decodes a FILE_ID_BOTH_DIR_INFORMATION
// entry (MS-FSCC 2.4.22). Its first 64 bytes are laid out identically to
// FILE_DIRECTORY_INFORMATION; the trailing fields add the short name and the
// server's 64-bit file reference number.
type FileIdBothDirectoryInformationDecoder []byte

func (c FileIdBothDirectoryInformationDecoder) IsInvalid() bool {
	if len(c) < 104 {
		return true
	}
	// FILE_ID_BOTH_DIR_INFORMATION timestamps must be nonnegative
	// ([MS-FSCC] 2.4.22).
	for _, timestamp := range []FiletimeDecoder{
		c.CreationTime(),
		c.LastAccessTime(),
		c.LastWriteTime(),
		c.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}
	// EndOfFile is signed but must be nonnegative ([MS-FSCC] 2.4.22).
	if c.EndOfFile() < 0 {
		return true
	}
	if c.AllocationSize() < 0 {
		return true
	}
	nameLength := uint64(c.FileNameLength())
	entrySize := 104 + nameLength
	if uint64(len(c)) < entrySize {
		return true
	}
	if IsInvalidDirectoryEntryName(c[104 : 104+nameLength]) {
		return true
	}
	shortLen := c.ShortNameLength()
	if shortLen > 24 || shortLen&1 != 0 {
		return true
	}
	if shortLen > 0 && IsInvalidShortName(c[70:70+shortLen]) {
		return true
	}
	next := uint64(c.NextEntryOffset())
	if next == 0 {
		return false
	}
	if next < entrySize || next >= uint64(len(c)) {
		return true
	}
	if Roundup(int(next), 8) != int(next) {
		return true
	}
	return uint64(len(c))-next < 104
}

func (c FileIdBothDirectoryInformationDecoder) NextEntryOffset() uint32 {
	return le.Uint32(c[:4])
}

// FileIndex is the resume cookie for restarting enumeration, not a file
// identifier; it is commonly 0. See FileId for the file reference number.
func (c FileIdBothDirectoryInformationDecoder) FileIndex() uint32 {
	return le.Uint32(c[4:8])
}

func (c FileIdBothDirectoryInformationDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(c[8:16])
}

func (c FileIdBothDirectoryInformationDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(c[16:24])
}

func (c FileIdBothDirectoryInformationDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(c[24:32])
}

func (c FileIdBothDirectoryInformationDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(c[32:40])
}

func (c FileIdBothDirectoryInformationDecoder) EndOfFile() int64 {
	return int64(le.Uint64(c[40:48]))
}

func (c FileIdBothDirectoryInformationDecoder) AllocationSize() int64 {
	return int64(le.Uint64(c[48:56]))
}

func (c FileIdBothDirectoryInformationDecoder) FileAttributes() uint32 {
	return le.Uint32(c[56:60])
}

func (c FileIdBothDirectoryInformationDecoder) FileNameLength() uint32 {
	return le.Uint32(c[60:64])
}

func (c FileIdBothDirectoryInformationDecoder) EaSize() uint32 {
	return le.Uint32(c[64:68])
}

func (c FileIdBothDirectoryInformationDecoder) ShortNameLength() uint8 {
	return c[68]
}

func (c FileIdBothDirectoryInformationDecoder) ShortName() string {
	n := c.ShortNameLength()
	return utf16le.DecodeToString(c[70 : 70+n])
}

// FileId is the server's 64-bit file reference number: the MFT record plus
// sequence number on NTFS, the inode number on Samba. Servers that cannot
// supply one report 0.
func (c FileIdBothDirectoryInformationDecoder) FileId() uint64 {
	return le.Uint64(c[96:104])
}

func (c FileIdBothDirectoryInformationDecoder) FileNameBytes() []byte {
	return c[104 : 104+c.FileNameLength()]
}

func (c FileIdBothDirectoryInformationDecoder) FileName() string {
	return utf16le.DecodeToString(c.FileNameBytes())
}

type FileRenameInformationType2Encoder struct {
	ReplaceIfExists uint8
	RootDirectory   uint64
	FileName        string
}

func (c *FileRenameInformationType2Encoder) Size() int {
	return 20 + utf16le.EncodedStringLen(c.FileName)
}

func (c *FileRenameInformationType2Encoder) Encode(p []byte) {
	flen := utf16le.EncodeString(p[20:], c.FileName)

	p[0] = c.ReplaceIfExists
	le.PutUint64(p[8:16], c.RootDirectory)
	le.PutUint32(p[16:20], uint32(flen))
}

type FileLinkInformationType2Encoder struct {
	ReplaceIfExists uint8
	RootDirectory   uint64
	FileName        string
}

func (c *FileLinkInformationType2Encoder) Size() int {
	return 20 + utf16le.EncodedStringLen(c.FileName)
}

func (c *FileLinkInformationType2Encoder) Encode(p []byte) {
	flen := utf16le.EncodeString(p[20:], c.FileName)

	p[0] = c.ReplaceIfExists
	le.PutUint64(p[8:16], c.RootDirectory)
	le.PutUint32(p[16:20], uint32(flen))
}

type FileDispositionInformationEncoder struct {
	DeletePending uint8
}

func (c *FileDispositionInformationEncoder) Size() int {
	return 1
}

func (c *FileDispositionInformationEncoder) Encode(p []byte) {
	p[0] = c.DeletePending
}

type FilePositionInformationEncoder struct {
	CurrentByteOffset int64
}

func (c *FilePositionInformationEncoder) Size() int {
	return 8
}

func (c *FilePositionInformationEncoder) Encode(p []byte) {
	le.PutUint64(p[:8], uint64(c.CurrentByteOffset))
}

type FileFsFullSizeInformationDecoder []byte

func (c FileFsFullSizeInformationDecoder) IsInvalid() bool {
	if len(c) < 32 {
		return true
	}

	// [MS-FSCC] 2.5.4 requires all three allocation-unit counts to be
	// non-negative signed 64-bit integers.
	return c.TotalAllocationUnits() < 0 ||
		c.CallerAvailableAllocationUnits() < 0 ||
		c.ActualAvailableAllocationUnits() < 0
}

func (c FileFsFullSizeInformationDecoder) TotalAllocationUnits() int64 {
	return int64(le.Uint64(c[:8]))
}

func (c FileFsFullSizeInformationDecoder) CallerAvailableAllocationUnits() int64 {
	return int64(le.Uint64(c[8:16]))
}

func (c FileFsFullSizeInformationDecoder) ActualAvailableAllocationUnits() int64 {
	return int64(le.Uint64(c[16:24]))
}

func (c FileFsFullSizeInformationDecoder) SectorsPerAllocationUnit() uint32 {
	return le.Uint32(c[24:28])
}

func (c FileFsFullSizeInformationDecoder) BytesPerSector() uint32 {
	return le.Uint32(c[28:32])
}

type FileQuotaInformationDecoder []byte

func (c FileQuotaInformationDecoder) IsInvalid() bool {
	if len(c) < 40 {
		return true
	}
	entrySize := 40 + uint64(c.SidLength())
	if uint64(len(c)) < entrySize {
		return true
	}
	// FILE_QUOTA_INFORMATION response timestamps and used quota must be
	// non-negative ([MS-FSCC] 2.4.41).
	if c.ChangeTime().HighDateTime()&0x80000000 != 0 || c.QuotaUsed() < 0 {
		return true
	}
	// QuotaThreshold MUST be >= 0 or -1 (no warning threshold).
	if c.QuotaThreshold() < -1 {
		return true
	}
	// QuotaLimit MUST be >= 0, -1 (no limit), or -2 (delete entry).
	if c.QuotaLimit() < -2 {
		return true
	}
	sid := c.Sid()
	if sid.IsInvalid() || uint64(c.SidLength()) != 8+4*uint64(sid.SubAuthorityCount()) {
		return true
	}
	next := uint64(c.NextEntryOffset())
	if next == 0 {
		return false
	}
	if next < entrySize || next >= uint64(len(c)) {
		return true
	}
	if Roundup(int(next), 8) != int(next) {
		return true
	}
	return uint64(len(c))-next < 40
}

func (c FileQuotaInformationDecoder) NextEntryOffset() uint32 {
	return le.Uint32(c[:4])
}

func (c FileQuotaInformationDecoder) SidLength() uint32 {
	return le.Uint32(c[4:8])
}

func (c FileQuotaInformationDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(c[8:16])
}

func (c FileQuotaInformationDecoder) QuotaUsed() int64 {
	return int64(le.Uint64(c[16:24]))
}

func (c FileQuotaInformationDecoder) QuotaThreshold() int64 {
	return int64(le.Uint64(c[24:32]))
}

func (c FileQuotaInformationDecoder) QuotaLimit() int64 {
	return int64(le.Uint64(c[32:40]))
}

func (c FileQuotaInformationDecoder) Sid() SidDecoder {
	return SidDecoder(c[40 : 40+c.SidLength()])
}

type FileEndOfFileInformationEncoder struct {
	EndOfFile int64
}

func (c *FileEndOfFileInformationEncoder) Size() int {
	return 8
}

func (c *FileEndOfFileInformationEncoder) Encode(p []byte) {
	le.PutUint64(p[:8], uint64(c.EndOfFile))
}

type FileEndOfFileInformationDecoder []byte

func (c FileEndOfFileInformationDecoder) IsInvalid() bool {
	if len(c) < 8 {
		return true
	}
	// EndOfFile is signed but must be nonnegative ([MS-FSCC] 2.4.13).
	return c.EndOfFile() < 0
}

func (c FileEndOfFileInformationDecoder) EndOfFile() int64 {
	return int64(le.Uint64(c[:8]))
}

type FileAllInformationDecoder []byte

func (c FileAllInformationDecoder) IsInvalid() bool {
	// FILE_ALL_INFORMATION includes NameInformation after the 96-byte fixed
	// prefix ([MS-FSCC] 2.4.2), and NameInformation starts with the
	// FileNameLength field ([MS-FSCC] 2.1.7).
	if len(c) < 100 {
		return true
	}

	return c.BasicInformation().IsInvalid() ||
		c.StandardInformation().IsInvalid() ||
		c.PositionInformation().IsInvalid() ||
		c.NameInformation().IsInvalid()
}

func (c FileAllInformationDecoder) BasicInformation() FileBasicInformationDecoder {
	return FileBasicInformationDecoder(c[:40])
}

func (c FileAllInformationDecoder) StandardInformation() FileStandardInformationDecoder {
	return FileStandardInformationDecoder(c[40:64])
}

func (c FileAllInformationDecoder) InternalInformation() FileInternalInformationDecoder {
	return FileInternalInformationDecoder(c[64:72])
}

func (c FileAllInformationDecoder) EaInformation() FileEaInformationDecoder {
	return FileEaInformationDecoder(c[72:76])
}

func (c FileAllInformationDecoder) AccessInformation() FileAccessInformationDecoder {
	return FileAccessInformationDecoder(c[76:80])
}

func (c FileAllInformationDecoder) PositionInformation() FilePositionInformationDecoder {
	return FilePositionInformationDecoder(c[80:88])
}

func (c FileAllInformationDecoder) ModeInformation() FileModeInformationDecoder {
	return FileModeInformationDecoder(c[88:92])
}

func (c FileAllInformationDecoder) AlignmentInformation() FileAlignmentInformationDecoder {
	return FileAlignmentInformationDecoder(c[92:96])
}

func (c FileAllInformationDecoder) NameInformation() FileNameInformationDecoder {
	return FileNameInformationDecoder(c[96:])
}

// FileAttributeTagInformationDecoder decodes FILE_ATTRIBUTE_TAG_INFORMATION
// ([MS-FSCC] 2.4.6). Unknown tags are preserved for the caller to classify.
type FileAttributeTagInformationDecoder []byte

func (c FileAttributeTagInformationDecoder) IsInvalid() bool        { return len(c) < 8 }
func (c FileAttributeTagInformationDecoder) FileAttributes() uint32 { return le.Uint32(c[:4]) }
func (c FileAttributeTagInformationDecoder) ReparseTag() uint32     { return le.Uint32(c[4:8]) }

type FileNetworkOpenInformationDecoder []byte

func (c FileNetworkOpenInformationDecoder) IsInvalid() bool {
	if len(c) < 56 {
		return true
	}
	for _, timestamp := range []FiletimeDecoder{
		c.CreationTime(),
		c.LastAccessTime(),
		c.LastWriteTime(),
		c.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}
	if c.EndOfFile() < 0 {
		return true
	}
	if c.AllocationSize() < 0 {
		return true
	}
	return false
}

func (c FileNetworkOpenInformationDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(c[:8])
}

func (c FileNetworkOpenInformationDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(c[8:16])
}

func (c FileNetworkOpenInformationDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(c[16:24])
}

func (c FileNetworkOpenInformationDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(c[24:32])
}

func (c FileNetworkOpenInformationDecoder) AllocationSize() int64 {
	return int64(le.Uint64(c[32:40]))
}

func (c FileNetworkOpenInformationDecoder) EndOfFile() int64 {
	return int64(le.Uint64(c[40:48]))
}

func (c FileNetworkOpenInformationDecoder) FileAttributes() uint32 {
	return le.Uint32(c[48:52])
}

type FileBasicInformationEncoder struct {
	CreationTime   Filetime
	LastAccessTime Filetime
	LastWriteTime  Filetime
	ChangeTime     Filetime
	FileAttributes uint32
}

func (c *FileBasicInformationEncoder) Size() int {
	return 40
}

func (c *FileBasicInformationEncoder) Encode(p []byte) {
	c.CreationTime.Encode(p[:8])
	c.LastAccessTime.Encode(p[8:16])
	c.LastWriteTime.Encode(p[16:24])
	c.ChangeTime.Encode(p[24:32])
	le.PutUint32(p[32:36], c.FileAttributes)
}

type FileBasicInformationDecoder []byte

func (c FileBasicInformationDecoder) IsInvalid() bool {
	if len(c) < 40 {
		return true
	}
	// FILE_BASIC_INFORMATION response timestamps are valid only when they are
	// nonnegative; -1 and -2 are reserved for setting file attributes
	// ([MS-FSCC] 2.4.7).
	for _, timestamp := range []FiletimeDecoder{
		c.CreationTime(),
		c.LastAccessTime(),
		c.LastWriteTime(),
		c.ChangeTime(),
	} {
		if timestamp.HighDateTime()&0x80000000 != 0 {
			return true
		}
	}
	return false
}

func (c FileBasicInformationDecoder) CreationTime() FiletimeDecoder {
	return FiletimeDecoder(c[:8])
}

func (c FileBasicInformationDecoder) LastAccessTime() FiletimeDecoder {
	return FiletimeDecoder(c[8:16])
}

func (c FileBasicInformationDecoder) LastWriteTime() FiletimeDecoder {
	return FiletimeDecoder(c[16:24])
}

func (c FileBasicInformationDecoder) ChangeTime() FiletimeDecoder {
	return FiletimeDecoder(c[24:32])
}

func (c FileBasicInformationDecoder) FileAttributes() uint32 {
	return le.Uint32(c[32:36])
}

type FileStandardInformationDecoder []byte

func (c FileStandardInformationDecoder) IsInvalid() bool {
	if len(c) < 24 {
		return true
	}
	// EndOfFile and AllocationSize are signed but must be nonnegative ([MS-FSCC] 2.4.41).
	return c.EndOfFile() < 0 || c.AllocationSize() < 0
}

func (c FileStandardInformationDecoder) AllocationSize() int64 {
	return int64(le.Uint64(c[:8]))
}

func (c FileStandardInformationDecoder) EndOfFile() int64 {
	return int64(le.Uint64(c[8:16]))
}

func (c FileStandardInformationDecoder) NumberOfLinks() uint32 {
	return le.Uint32(c[16:20])
}

func (c FileStandardInformationDecoder) DeletePending() uint8 {
	return c[20]
}

func (c FileStandardInformationDecoder) Directory() uint8 {
	return c[21]
}

type FileInternalInformationDecoder []byte

func (c FileInternalInformationDecoder) IsInvalid() bool {
	return len(c) < 8
}

func (c FileInternalInformationDecoder) IndexNumber() int64 {
	return int64(le.Uint64(c[:8]))
}

type FileEaInformationDecoder []byte

func (c FileEaInformationDecoder) IsInvalid() bool {
	return len(c) < 4
}

func (c FileEaInformationDecoder) EaSize() uint32 {
	return le.Uint32(c[:4])
}

type FileAccessInformationDecoder []byte

func (c FileAccessInformationDecoder) IsInvalid() bool {
	return len(c) < 4
}

func (c FileAccessInformationDecoder) AccessFlags() uint32 {
	return le.Uint32(c[:4])
}

type FilePositionInformationDecoder []byte

func (c FilePositionInformationDecoder) IsInvalid() bool {
	if len(c) < 8 {
		return true
	}
	// CurrentByteOffset is signed but must be nonnegative ([MS-FSCC] 2.4.32).
	return c.CurrentByteOffset() < 0
}

func (c FilePositionInformationDecoder) CurrentByteOffset() int64 {
	return int64(le.Uint64(c[:8]))
}

type FileModeInformationDecoder []byte

func (c FileModeInformationDecoder) IsInvalid() bool {
	return len(c) < 4
}

func (c FileModeInformationDecoder) Mode() uint32 {
	return le.Uint32(c[:4])
}

type FileAlignmentInformationDecoder []byte

func (c FileAlignmentInformationDecoder) IsInvalid() bool {
	return len(c) < 4
}

func (c FileAlignmentInformationDecoder) AlignmentRequirement() uint32 {
	return le.Uint32(c[:4])
}

type FileNameInformationDecoder []byte

func (c FileNameInformationDecoder) IsInvalid() bool {
	if len(c) < 4 {
		return true
	}

	nameLength := uint64(c.FileNameLength())
	if nameLength&1 != 0 || uint64(len(c)) < 4+nameLength {
		return true
	}

	if nameLength > 0 && IsInvalidPathname(c[4:4+nameLength]) {
		return true
	}

	return false
}

func (c FileNameInformationDecoder) FileNameLength() uint32 {
	return le.Uint32(c[:4])
}

func (c FileNameInformationDecoder) FileNameBytes() []byte {
	return c[4 : 4+int(c.FileNameLength())]
}

func (c FileNameInformationDecoder) FileName() string {
	return utf16le.DecodeToString(c.FileNameBytes())
}
