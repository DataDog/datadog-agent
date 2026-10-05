package security

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// validateResourceClaim validates CLAIM_SECURITY_ATTRIBUTE_RELATIVE_V1
// ([MS-DTYP] 2.4.10.1), whose offsets are relative to the claim, not the ACE.
func validateResourceClaim(claim []byte) error {
	invalid := func() error { return fmt.Errorf("invalid resource attribute claim") }
	if len(claim) < 16 {
		return invalid()
	}
	valueType := binary.LittleEndian.Uint16(claim[4:6])
	switch valueType {
	case 1, 2, 3, 5, 6, 0x10:
	default:
		return invalid()
	}
	// Reserved is ignored on receipt. Upper flag bits are application-defined,
	// except that manual and policy-derived provenance are mutually exclusive.
	flags := binary.LittleEndian.Uint32(claim[8:12])
	if flags&0xffc0 != 0 || flags&0x30000 == 0x30000 {
		return invalid()
	}
	if flags&2 != 0 && valueType != 3 && valueType != 5 {
		return invalid()
	}
	count := uint64(binary.LittleEndian.Uint32(claim[12:16]))
	if count > uint64((len(claim)-16)/4) {
		return invalid()
	}
	valuesEnd := 16 + int(count)*4
	contains := func(offset uint32, length int) bool {
		return uint64(offset) >= uint64(valuesEnd) && uint64(offset)+uint64(length) <= uint64(len(claim))
	}

	// Record valid NUL-terminated UTF-16 suffixes in one pass. Repeated or
	// overlapping string offsets must not cause quadratic validation work.
	terminated := make([]bool, len(claim)+2)
	for i := len(claim) - 2; i >= valuesEnd; i-- {
		u := binary.LittleEndian.Uint16(claim[i : i+2])
		switch {
		case u == 0:
			terminated[i] = true
		case u < 0xd800 || u > 0xdfff:
			terminated[i] = terminated[i+2]
		case u <= 0xdbff && i+4 <= len(claim):
			v := binary.LittleEndian.Uint16(claim[i+2 : i+4])
			terminated[i] = v >= 0xdc00 && v <= 0xdfff && terminated[i+4]
		}
	}
	name := binary.LittleEndian.Uint32(claim[:4])
	if !contains(name, 4) || !terminated[name] || binary.LittleEndian.Uint16(claim[name:name+2]) == 0 {
		return invalid()
	}
	for i := 16; i < valuesEnd; i += 4 {
		offset := binary.LittleEndian.Uint32(claim[i : i+4])
		switch valueType {
		case 1, 2, 6:
			if !contains(offset, 8) {
				return invalid()
			}
			if valueType == 6 && binary.LittleEndian.Uint64(claim[offset:offset+8]) > 1 {
				return invalid()
			}
		case 3:
			if !contains(offset, 2) || !terminated[offset] {
				return invalid()
			}
		case 5, 0x10:
			if !contains(offset, 4) {
				return invalid()
			}
			length := uint64(binary.LittleEndian.Uint32(claim[offset : offset+4]))
			start := uint64(offset) + 4
			if length > uint64(len(claim))-start {
				return invalid()
			}
			if valueType == 5 {
				if length > uint64(len("S-1-0x")+12+maxSubAuthorities*11+1) {
					return invalid()
				}
				// SID claims contain a SID string, not a binary SID. The
				// octet string may include its terminating NUL byte.
				value := strings.TrimSuffix(string(claim[start:start+length]), "\x00")
				if _, err := ParseSID(value); err != nil {
					return invalid()
				}
			}
		}
	}
	return nil
}
