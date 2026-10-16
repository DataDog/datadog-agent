package utf16le

import (
	"encoding/binary"
	"unicode/utf16"
	"unicode/utf8"
)

var (
	le = binary.LittleEndian
)

func EncodedStringLen(s string) int {
	l := 0
	for _, r := range s {
		if 0x10000 <= r && r <= '\U0010FFFF' {
			l += 4
		} else {
			l += 2
		}
	}
	return l
}

func EncodeString(dst []byte, src string) int {
	n := 0
	for _, r := range src {
		if r <= 0xffff {
			le.PutUint16(dst[n:], uint16(r))
			n += 2
		} else {
			r1, r2 := utf16.EncodeRune(r)
			le.PutUint16(dst[n:], uint16(r1))
			le.PutUint16(dst[n+2:], uint16(r2))
			n += 4
		}
	}
	return n
}

func EncodeStringToBytes(s string) []byte {
	if len(s) == 0 {
		return nil
	}
	bs := make([]byte, EncodedStringLen(s))
	EncodeString(bs, s)
	return bs
}

func DecodeToString(bs []byte) string {
	n := len(bs) / 2
	if n == 0 {
		return ""
	}
	if le.Uint16(bs[2*(n-1):2*n]) == 0 {
		n--
	}
	if n == 0 {
		return ""
	}

	buf := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		u := le.Uint16(bs[2*i : 2*i+2])
		if u < 0xd800 || u >= 0xe000 {
			if u < 0x80 {
				buf = append(buf, byte(u))
			} else {
				var b [utf8.UTFMax]byte
				m := utf8.EncodeRune(b[:], rune(u))
				buf = append(buf, b[:m]...)
			}
		} else if u <= 0xdbff && i+1 < n {
			u2 := le.Uint16(bs[2*(i+1) : 2*(i+2)])
			if u2 >= 0xdc00 && u2 <= 0xdfff {
				r := utf16.DecodeRune(rune(u), rune(u2))
				var b [utf8.UTFMax]byte
				m := utf8.EncodeRune(b[:], r)
				buf = append(buf, b[:m]...)
				i++
			} else {
				var b [utf8.UTFMax]byte
				m := utf8.EncodeRune(b[:], utf8.RuneError)
				buf = append(buf, b[:m]...)
			}
		} else {
			var b [utf8.UTFMax]byte
			m := utf8.EncodeRune(b[:], utf8.RuneError)
			buf = append(buf, b[:m]...)
		}
	}
	return string(buf)
}
