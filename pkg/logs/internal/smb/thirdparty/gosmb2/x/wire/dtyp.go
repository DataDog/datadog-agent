// ref: MS-DTYP

package wire

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Filetime struct {
	LowDateTime  uint32
	HighDateTime uint32
}

// FILETIME is an unsigned count of 100-nanosecond intervals, so split the
// value before converting it to time.Unix ([MS-DTYP] 2.3.3).
func filetimeToTime(ticks uint64) time.Time {
	return time.Unix(int64(ticks/10000000)-11644473600, int64(ticks%10000000)*100)
}

func (ft Filetime) Size() int {
	return 8
}

func (ft Filetime) Encode(p []byte) {
	le.PutUint32(p[:4], ft.LowDateTime)
	le.PutUint32(p[4:8], ft.HighDateTime)
}

func (ft Filetime) Time() time.Time {
	return filetimeToTime(uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime))
}

// TimeToFiletime converts t to FILETIME. A zero time produces a zero FILETIME,
// which leaves timestamps unchanged in a file attribute update. The bool is
// false when a nonzero time is outside the representable range.
func TimeToFiletime(t time.Time) (Filetime, bool) {
	if t.IsZero() {
		return Filetime{}, true
	}

	const unixToFiletimeSeconds = int64(11644473600)
	const maxFiletimeSeconds = int64(^uint64(0) / 10000000)
	seconds := t.Unix()
	if seconds < -unixToFiletimeSeconds || seconds > maxFiletimeSeconds-unixToFiletimeSeconds {
		return Filetime{}, false
	}

	filetimeSeconds := uint64(seconds + unixToFiletimeSeconds)
	nanoseconds := uint64(t.Nanosecond() / 100)
	if filetimeSeconds > (^uint64(0)-nanoseconds)/10000000 {
		return Filetime{}, false
	}

	filetime := filetimeSeconds*10000000 + nanoseconds
	return Filetime{
		LowDateTime:  uint32(filetime),
		HighDateTime: uint32(filetime >> 32),
	}, true
}

type FiletimeDecoder []byte

func (ft FiletimeDecoder) IsInvalid() bool {
	return len(ft) < 8
}

func (ft FiletimeDecoder) LowDateTime() uint32 {
	return le.Uint32(ft[:4])
}

func (ft FiletimeDecoder) HighDateTime() uint32 {
	return le.Uint32(ft[4:8])
}

func (ft FiletimeDecoder) Time() time.Time {
	return filetimeToTime(uint64(ft.HighDateTime())<<32 | uint64(ft.LowDateTime()))
}

func (ft FiletimeDecoder) Decode() Filetime {
	return Filetime{
		LowDateTime:  ft.LowDateTime(),
		HighDateTime: ft.HighDateTime(),
	}
}

type Sid struct {
	Revision            uint8
	IdentifierAuthority uint64
	SubAuthority        []uint32
}

func (sid *Sid) String() string {
	if sid == nil {
		return "<nil>"
	}
	list := make([]string, 0, 3+len(sid.SubAuthority))
	list = append(list, "S")
	list = append(list, strconv.Itoa(int(sid.Revision)))
	if sid.IdentifierAuthority < uint64(1<<32) {
		list = append(list, strconv.FormatUint(sid.IdentifierAuthority, 10))
	} else {
		list = append(list, fmt.Sprintf("0x%012x", sid.IdentifierAuthority))
	}
	for _, a := range sid.SubAuthority {
		list = append(list, strconv.FormatUint(uint64(a), 10))
	}
	return strings.Join(list, "-")
}

func (sid *Sid) Size() int {
	if sid == nil {
		return 0
	}
	return 8 + len(sid.SubAuthority)*4
}

func (sid *Sid) Encode(p []byte) {
	if sid == nil || len(p) < sid.Size() {
		return
	}
	p[0] = sid.Revision
	p[1] = uint8(len(sid.SubAuthority))
	for j := range 6 {
		p[2+j] = byte(sid.IdentifierAuthority >> uint64(8*(5-j)))
	}
	off := 8
	for _, u := range sid.SubAuthority {
		le.PutUint32(p[off:off+4], u)
		off += 4
	}
}

type SidDecoder []byte

func (c SidDecoder) IsInvalid() bool {
	if len(c) < 8 {
		return true
	}

	if c.Revision() != 1 || c.SubAuthorityCount() > 15 {
		return true
	}

	if uint64(len(c)) < 8+4*uint64(c.SubAuthorityCount()) {
		return true
	}

	return false
}

func (c SidDecoder) Revision() uint8 {
	return c[0]
}

func (c SidDecoder) SubAuthorityCount() uint8 {
	return c[1]
}

func (c SidDecoder) IdentifierAuthority() uint64 {
	var u uint64
	for j := range 6 {
		u += uint64(c[7-j]) << uint64(8*j)
	}
	return u
}

func (c SidDecoder) SubAuthority() []uint32 {
	count := c.SubAuthorityCount()
	as := make([]uint32, count)
	off := 8
	for i := range count {
		as[i] = le.Uint32(c[off : off+4])
		off += 4
	}
	return as
}

func (c SidDecoder) Decode() *Sid {
	return &Sid{
		Revision:            c.Revision(),
		IdentifierAuthority: c.IdentifierAuthority(),
		SubAuthority:        c.SubAuthority(),
	}
}

// ----------------------------------------------------------------------------
// [MS-DTYP] 2.2.57 UNC Validation Helpers
// ----------------------------------------------------------------------------

func isPchar(ch byte) bool {
	if ch <= 0x1F {
		return false
	}
	switch ch {
	case '"', '\\', '/', '[', ']', ':', '|', '<', '>', '+', '=', ';', ',', '*', '?':
		return false
	default:
		return true
	}
}

func isInvalidHostName(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 || len(b)/2 > 255 || IsDotDirectoryName(b) {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		if b[i+1] == 0 {
			ch := b[i]
			if ch <= 0x1F || ch == ' ' {
				return true
			}
			switch ch {
			case '"', '\\', '/', '|', '<', '>', '*', '?':
				return true
			}
		}
	}
	return false
}

// IsInvalidShareName reports whether b is an invalid share name
// ([MS-FSCC] 2.1.6, [MS-DTYP] 2.2.57).
func IsInvalidShareName(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 || len(b)/2 > 80 || IsDotDirectoryName(b) {
		return true
	}
	for i := 0; i < len(b); i += 2 {
		if b[i+1] == 0 && !isPchar(b[i]) {
			return true
		}
	}
	return false
}

// TrimUNCPrefix strips a leading UNC prefix ("\\", "\??\UNC\", or "\\?\UNC\")
// from b and reports whether a prefix was found.
func TrimUNCPrefix(b []byte) ([]byte, bool) {
	if len(b) >= 16 && b[0] == '\\' && b[1] == 0 &&
		(b[2] == '?' || b[2] == '\\') && b[3] == 0 &&
		b[4] == '?' && b[5] == 0 &&
		b[6] == '\\' && b[7] == 0 &&
		(b[8] == 'U' || b[8] == 'u') && b[9] == 0 &&
		(b[10] == 'N' || b[10] == 'n') && b[11] == 0 &&
		(b[12] == 'C' || b[12] == 'c') && b[13] == 0 &&
		b[14] == '\\' && b[15] == 0 {
		return b[16:], true
	}
	if len(b) >= 4 && b[0] == '\\' && b[1] == 0 && b[2] == '\\' && b[3] == 0 {
		if len(b) >= 6 && (b[4] == '?' || b[4] == '.') && b[5] == 0 {
			return b, false
		}
		return b[4:], true
	}
	return b, false
}

// HasUNCPrefix reports whether b begins with a UNC prefix ("\\", "\??\UNC\", or "\\?\UNC\").
func HasUNCPrefix(b []byte) bool {
	_, ok := TrimUNCPrefix(b)
	return ok
}

func parseUNC(b []byte) (host, share, object []byte, hasSlash bool, ok bool) {
	hostEnd := -1
	for i := 0; i < len(b); i += 2 {
		if b[i] == '\\' && b[i+1] == 0 {
			hostEnd = i
			break
		}
	}
	if hostEnd <= 0 {
		return nil, nil, nil, false, false
	}
	host = b[:hostEnd]
	rem := b[hostEnd+2:]
	if len(rem) == 0 {
		return nil, nil, nil, false, false
	}

	shareEnd := -1
	for i := 0; i < len(rem); i += 2 {
		if rem[i] == '\\' && rem[i+1] == 0 {
			shareEnd = i
			break
		}
	}
	if shareEnd == -1 {
		return host, rem, nil, false, true
	}
	if shareEnd == 0 {
		return nil, nil, nil, false, false
	}
	share = rem[:shareEnd]
	object = rem[shareEnd+2:]
	return host, share, object, true, true
}

// IsInvalidSharePath reports whether b is an invalid full share path
// ([MS-SMB2] 2.2.9), formatted as "\\server\share".
func IsInvalidSharePath(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) < 8 || len(b)/2 > 338 {
		return true
	}
	if b[0] != '\\' || b[1] != 0 || b[2] != '\\' || b[3] != 0 {
		return true
	}
	if len(b) >= 6 && (b[4] == '?' || b[4] == '.') && b[5] == 0 {
		return true
	}
	host, share, object, hasSlash, ok := parseUNC(b[4:])
	if !ok || hasSlash || len(object) > 0 {
		return true
	}
	return isInvalidHostName(host) || IsInvalidShareName(share)
}

// IsInvalidUNC reports whether b is an invalid UNC pathname
// ([MS-DTYP] 2.2.57), formatted as "\\host\share[\object]".
func IsInvalidUNC(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) < 8 || len(b)/2 > 32760 {
		return true
	}
	rem, ok := TrimUNCPrefix(b)
	if !ok {
		return true
	}
	host, share, object, hasSlash, ok := parseUNC(rem)
	if !ok {
		return true
	}
	if isInvalidHostName(host) || IsInvalidShareName(share) {
		return true
	}
	if hasSlash {
		if len(object) == 0 {
			return true
		}
		return IsInvalidRelativePathname(object)
	}
	return false
}

func isDriveLetter(b []byte) bool {
	if len(b) < 4 || b[1] != 0 || b[3] != 0 {
		return false
	}
	ch := b[0]
	if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')) {
		return false
	}
	return b[2] == ':'
}

func hasDevicePrefix(b []byte) bool {
	return len(b) >= 8 && b[0] == '\\' && b[1] == 0 &&
		(b[2] == '?' || b[2] == '\\') && b[3] == 0 &&
		b[4] == '?' && b[5] == 0 &&
		b[6] == '\\' && b[7] == 0
}

func isInvalidSubstitutePathname(b []byte) bool {
	if isInvalidUTF16LE(b) || len(b) == 0 || len(b)/2 > 32760 {
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
			if !IsDotDirectoryName(comp) && IsInvalidDirectoryEntryName(comp) {
				return true
			}
			start = i + 2
		}
	}
	last := b[start:]
	if !IsDotDirectoryName(last) && IsInvalidPathnameComponent(last) {
		return true
	}
	return false
}

// isInvalidSubstituteName validates a symbolic link substitute name against
// [MS-SMB2] 2.2.2.2.1 and [MS-FSCC] 2.1.2.4:
//   - An empty substitute name is invalid.
//   - When SYMLINK_FLAG_RELATIVE is set, the path is relative to the directory
//     containing the symbolic link and MUST NOT start with "\" or "/". It must
//     not include NT/Win32 device prefixes ("\??\", "\\?\"), UNC prefixes
//     ("\\"), or drive letters ("C:").
//   - When SYMLINK_FLAG_RELATIVE is not set (absolute link), the substitute name
//     must be an absolute path: a UNC path ("\\server\share\..."), an NT or
//     Win32 device drive path ("\??\C:\...", "\\?\C:\..."), a local drive path
//     ("C:\..."), or a volume-rooted path ("\dir\file"). Un-rooted relative
//     paths (e.g., "dir\target") are invalid when Flags == 0.
//     Note: for remote SMB2 error responses ([MS-SMB2] 2.2.2.2.1), the server
//     SHOULD NOT return an absolute target that is a local resource, and
//     SymbolicLinkErrorResponseDecoder.IsInvalid rejects non-UNC paths; local
//     filesystem reparse buffers ([MS-FSCC] 2.1.2.4) permit local drive and
//     volume-rooted targets.
func isInvalidSubstituteName(sub []byte, flags uint32) bool {
	// [MS-SMB2] 2.2.2.2.1: SubstituteName MUST NOT be empty.
	if len(sub) == 0 {
		return true
	}
	if flags&SYMLINK_FLAG_RELATIVE != 0 {
		if hasDevicePrefix(sub) || HasUNCPrefix(sub) || isDriveLetter(sub) {
			return true
		}
		if len(sub) >= 2 && sub[1] == 0 && (sub[0] == '\\' || sub[0] == '/') {
			return true
		}
		return isInvalidSubstitutePathname(sub)
	}
	if rem, ok := TrimUNCPrefix(sub); ok {
		if len(rem) == 0 {
			return true
		}
		host, share, object, hasSlash, ok := parseUNC(rem)
		if !ok || isInvalidHostName(host) || IsInvalidShareName(share) {
			return true
		}
		if hasSlash {
			if len(object) == 0 {
				return true
			}
			return isInvalidSubstitutePathname(object)
		}
		return false
	}
	rem := sub
	if hasDevicePrefix(rem) {
		rem = rem[8:]
		if len(rem) == 0 {
			return true
		}
		if !isDriveLetter(rem) {
			return true
		}
	}
	if isDriveLetter(rem) {
		rem = rem[4:]
		if len(rem) >= 2 && rem[0] == '\\' && rem[1] == 0 {
			rem = rem[2:]
		}
		if len(rem) == 0 {
			return false
		}
		return isInvalidSubstitutePathname(rem)
	}
	if len(sub) >= 2 && sub[0] == '\\' && sub[1] == 0 {
		return isInvalidSubstitutePathname(sub)
	}
	return true
}

// normalizeSymlinkTarget normalizes symbolic link target paths by stripping
// LongNamePrefix ("\\?\", "\??\") and converting device UNC prefixes
// ("\\?\UNC\", "\??\UNC\") to standard UNC ("\\") format.
func normalizeSymlinkTarget(target string) string {
	switch {
	case len(target) >= 8 && (strings.EqualFold(target[:8], `\\?\UNC\`) || strings.EqualFold(target[:8], `\??\UNC\`)):
		return `\\` + target[8:]
	case len(target) >= 4 && (strings.EqualFold(target[:4], `\??\`) || strings.EqualFold(target[:4], `\\?\`)):
		return target[4:]
	default:
		return target
	}
}
