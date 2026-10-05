package msrpc

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/smb/thirdparty/gosmb2/internal/utf16le"
)

var (
	errBufferTooSmall = errors.New("ndr: buffer too small")
	errInvalidString  = errors.New("ndr: invalid string encoding")
	errInvalidCount   = errors.New("ndr: invalid element count")
	errInvalidOffset  = errors.New("ndr: invalid offset")
)

// Encoder represents an NDR 32 stream encoder.
type Encoder struct {
	buf []byte
}

func NewEncoder() *Encoder {
	return &Encoder{buf: make([]byte, 0, 128)}
}

func (e *Encoder) Len() int {
	return len(e.buf)
}

func (e *Encoder) Bytes() []byte {
	return e.buf
}

func (e *Encoder) Align(n int) {
	if n <= 1 {
		return
	}
	rem := len(e.buf) % n
	if rem != 0 {
		pad := n - rem
		for range pad {
			e.buf = append(e.buf, 0)
		}
	}
}

func (e *Encoder) WriteUint8(v uint8) {
	e.buf = append(e.buf, v)
}

func (e *Encoder) WriteUint16(v uint16) {
	e.Align(2)
	e.buf = binary.LittleEndian.AppendUint16(e.buf, v)
}

func (e *Encoder) WriteUint32(v uint32) {
	e.Align(4)
	e.buf = binary.LittleEndian.AppendUint32(e.buf, v)
}

func (e *Encoder) WriteBytes(b []byte) {
	e.buf = append(e.buf, b...)
}

// WriteConformantVaryingString writes a null-terminated UTF-16LE conformant and varying string.
func (e *Encoder) WriteConformantVaryingString(s string) {
	e.Align(4)
	// UTF-16 characters count including null terminator
	count := uint32(utf16le.EncodedStringLen(s)/2 + 1)
	e.WriteUint32(count) // MaxCount
	e.WriteUint32(0)     // Offset
	e.WriteUint32(count) // ActualCount

	strBuf := make([]byte, count*2)
	utf16le.EncodeString(strBuf, s)
	e.WriteBytes(strBuf)
	e.Align(4)
}

// Decoder represents an NDR 32 stream decoder.
type Decoder struct {
	buf []byte
	off int
}

func NewDecoder(buf []byte) *Decoder {
	return &Decoder{buf: buf, off: 0}
}

func (d *Decoder) Offset() int {
	return d.off
}

func (d *Decoder) Remaining() int {
	if d.off >= len(d.buf) {
		return 0
	}
	return len(d.buf) - d.off
}

func (d *Decoder) Align(n int) error {
	if n <= 1 {
		return nil
	}
	rem := d.off % n
	if rem != 0 {
		pad := n - rem
		if pad > len(d.buf)-d.off {
			return errBufferTooSmall
		}
		d.off += pad
	}
	return nil
}

func (d *Decoder) ReadUint8() (uint8, error) {
	if len(d.buf)-d.off < 1 {
		return 0, errBufferTooSmall
	}
	v := d.buf[d.off]
	d.off++
	return v, nil
}

func (d *Decoder) ReadUint16() (uint16, error) {
	if err := d.Align(2); err != nil {
		return 0, err
	}
	if len(d.buf)-d.off < 2 {
		return 0, errBufferTooSmall
	}
	v := binary.LittleEndian.Uint16(d.buf[d.off : d.off+2])
	d.off += 2
	return v, nil
}

func (d *Decoder) ReadUint32() (uint32, error) {
	if err := d.Align(4); err != nil {
		return 0, err
	}
	if len(d.buf)-d.off < 4 {
		return 0, errBufferTooSmall
	}
	v := binary.LittleEndian.Uint32(d.buf[d.off : d.off+4])
	d.off += 4
	return v, nil
}

func (d *Decoder) ReadBytes(n int) ([]byte, error) {
	if n < 0 || n > len(d.buf)-d.off {
		return nil, errBufferTooSmall
	}
	res := d.buf[d.off : d.off+n]
	d.off += n
	return res, nil
}

// ReadConformantVaryingString decodes a null-terminated UTF-16LE conformant and varying string.
func (d *Decoder) ReadConformantVaryingString() (string, error) {
	if err := d.Align(4); err != nil {
		return "", err
	}
	maxCount, err := d.ReadUint32()
	if err != nil {
		return "", err
	}
	offset, err := d.ReadUint32()
	if err != nil {
		return "", err
	}
	actualCount, err := d.ReadUint32()
	if err != nil {
		return "", err
	}

	if offset != 0 {
		return "", fmt.Errorf("%w: non-zero offset %d", errInvalidOffset, offset)
	}
	if actualCount > maxCount {
		return "", fmt.Errorf("%w: actualCount %d > maxCount %d", errInvalidCount, actualCount, maxCount)
	}
	if actualCount > 65536 {
		return "", fmt.Errorf("%w: string length %d exceeds max limit", errInvalidCount, actualCount)
	}

	byteLen := int(actualCount) * 2
	if byteLen > len(d.buf)-d.off {
		return "", errBufferTooSmall
	}

	raw := d.buf[d.off : d.off+byteLen]
	d.off += byteLen
	_ = d.Align(4)

	// [string] wchar_t* fields in [MS-SRVS] sections 2.2.4.23 and 2.2.4.26
	// are null-terminated UTF-16 strings.
	if actualCount == 0 || binary.LittleEndian.Uint16(raw[len(raw)-2:]) != 0 {
		return "", errInvalidString
	}
	for i := 0; i < len(raw)-2; i += 2 {
		u := binary.LittleEndian.Uint16(raw[i : i+2])
		if u == 0 {
			return "", errInvalidString
		}
		if 0xd800 <= u && u <= 0xdbff {
			if i+4 > len(raw)-2 {
				return "", errInvalidString
			}
			low := binary.LittleEndian.Uint16(raw[i+2 : i+4])
			if low < 0xdc00 || low > 0xdfff {
				return "", errInvalidString
			}
			i += 2
		} else if 0xdc00 <= u && u <= 0xdfff {
			return "", errInvalidString
		}
	}

	str := utf16le.DecodeToString(raw)
	return str, nil
}
