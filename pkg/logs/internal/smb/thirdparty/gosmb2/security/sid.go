package security

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// SID is a Windows security identifier.
type SID struct {
	Revision            uint8
	IdentifierAuthority uint64
	SubAuthority        []uint32
}

func (sid *SID) String() string {
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
	for _, sub := range sid.SubAuthority {
		list = append(list, strconv.FormatUint(uint64(sub), 10))
	}
	return strings.Join(list, "-")
}

func (sid *SID) Size() int {
	if sid == nil {
		return 0
	}
	return 8 + 4*len(sid.SubAuthority)
}

func (sid *SID) Encode(p []byte) {
	if sid == nil || len(p) < sid.Size() {
		return
	}
	p[0] = sid.Revision
	p[1] = byte(len(sid.SubAuthority))
	p[2] = byte(sid.IdentifierAuthority >> 40)
	p[3] = byte(sid.IdentifierAuthority >> 32)
	p[4] = byte(sid.IdentifierAuthority >> 24)
	p[5] = byte(sid.IdentifierAuthority >> 16)
	p[6] = byte(sid.IdentifierAuthority >> 8)
	p[7] = byte(sid.IdentifierAuthority)
	off := 8
	for _, sub := range sid.SubAuthority {
		binary.LittleEndian.PutUint32(p[off:off+4], sub)
		off += 4
	}
}

func (sid *SID) validate() error {
	if sid == nil {
		return nil
	}
	if sid.Revision != 1 || len(sid.SubAuthority) > maxSubAuthorities || sid.IdentifierAuthority > 0xffffffffffff {
		return fmt.Errorf("invalid SID")
	}
	return nil
}

const (
	maxSubAuthorities = 15
)

// ParseSID parses the SID string format defined by [MS-DTYP] section 2.4.2.1.
func ParseSID(s string) (*SID, error) {
	rest, ok := strings.CutPrefix(s, "S-1-")
	if !ok {
		return nil, fmt.Errorf("invalid SID %q", s)
	}
	parts := strings.Split(rest, "-")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid SID %q", s)
	}
	if len(parts)-1 > maxSubAuthorities {
		return nil, fmt.Errorf("invalid SID %q: too many subauthorities", s)
	}

	authority, err := parseIdentifierAuthority(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid SID %q: %w", s, err)
	}

	subAuthorities := make([]uint32, len(parts)-1)
	for i, part := range parts[1:] {
		value, err := parseDecimal(part, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid SID %q: subauthority %d: %w", s, i, err)
		}
		subAuthorities[i] = uint32(value)
	}

	return &SID{
		Revision:            1,
		IdentifierAuthority: authority,
		SubAuthority:        subAuthorities,
	}, nil
}

func parseIdentifierAuthority(s string) (uint64, error) {
	hex, isHex := strings.CutPrefix(s, "0x")
	if !isHex {
		hex, isHex = strings.CutPrefix(s, "0X")
	}
	if isHex {
		if len(s) != 14 {
			return 0, fmt.Errorf("identifier authority must contain 12 hexadecimal digits")
		}
		value, err := strconv.ParseUint(hex, 16, 48)
		if err != nil {
			return 0, fmt.Errorf("invalid identifier authority")
		}
		if value < 1<<32 {
			return 0, fmt.Errorf("identifier authority below 2^32 must be decimal")
		}
		return value, nil
	}

	value, err := parseDecimal(s, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid identifier authority")
	}
	return value, nil
}

func parseDecimal(s string, bitSize int) (uint64, error) {
	if s == "" || len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("invalid decimal number")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid decimal number")
		}
	}
	value, err := strconv.ParseUint(s, 10, bitSize)
	if err != nil {
		return 0, fmt.Errorf("decimal number is out of range")
	}
	return value, nil
}

// MustSID is like ParseSID but panics if s is not a valid SID.
func MustSID(s string) *SID {
	sid, err := ParseSID(s)
	if err != nil {
		panic(err)
	}
	return sid
}
