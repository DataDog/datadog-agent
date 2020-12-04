// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package pb

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/tinylib/msgp/msgp"
)

// repairUTF8 ensures all characters in s are UTF-8 by replacing non-UTF-8 characters
// with the replacement char �
func repairUTF8(s string) string {
	in := strings.NewReader(s)
	var out bytes.Buffer
	out.Grow(len(s))

	for {
		r, _, err := in.ReadRune()
		if err != nil {
			// note: by contract, if `in` contains non-valid utf-8, no error is returned. Rather the utf-8 replacement
			// character is returned. Therefore, the only error should usually be io.EOF indicating end of string.
			// If any other error is returned by chance, we quit as well, outputting whatever part of the string we
			// had already constructed.
			return out.String()
		}

		out.WriteRune(r)
	}
}

// parseString reads the next type in the msgpack payload and
// converts the BinType or the StrType in a valid string.
func parseString(dc *msgp.Reader) (string, error) {
	// read the generic representation type without decoding
	t, err := dc.NextType()
	if err != nil {
		return "", err
	}
	switch t {
	case msgp.BinType:
		i, err := dc.ReadBytes(nil)
		if err != nil {
			return "", err
		}
		if utf8.Valid(i) {
			return msgp.UnsafeString(i), nil
		}
		return repairUTF8(msgp.UnsafeString(i)), nil
	case msgp.StrType:
		i, err := dc.ReadString()
		if err != nil {
			return "", err
		}
		if utf8.ValidString(i) {
			return i, nil
		}
		return repairUTF8(i), nil
	default:
		return "", msgp.TypeError{Encoded: t, Method: msgp.StrType}
	}
}

// parseFloat64 parses a float64 even if the sent value is an int64 or an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseFloat64(dc *msgp.Reader) (float64, error) {
	// read the generic representation type without decoding
	t, err := dc.NextType()
	if err != nil {
		return 0, err
	}

	switch t {
	case msgp.IntType:
		i, err := dc.ReadInt64()
		if err != nil {
			return 0, err
		}

		return float64(i), nil
	case msgp.UintType:
		i, err := dc.ReadUint64()
		if err != nil {
			return 0, err
		}

		return float64(i), nil
	case msgp.Float64Type:
		f, err := dc.ReadFloat64()
		if err != nil {
			return 0, err
		}

		return f, nil
	default:
		return 0, msgp.TypeError{Encoded: t, Method: msgp.Float64Type}
	}
}

// cast to int64 values that are int64 but that are sent in uint64
// over the wire. Set to 0 if they overflow the MaxInt64 size. This
// cast should be used ONLY while decoding int64 values that are
// sent as uint64 to reduce the payload size, otherwise the approach
// is not correct in the general sense.
func castInt64(v uint64) (int64, bool) {
	if v > math.MaxInt64 {
		return 0, false
	}

	return int64(v), true
}

// parseInt64 parses an int64 even if the sent value is an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt64(dc *msgp.Reader) (int64, error) {
	// read the generic representation type without decoding
	t, err := dc.NextType()
	if err != nil {
		return 0, err
	}

	switch t {
	case msgp.IntType:
		i, err := dc.ReadInt64()
		if err != nil {
			return 0, err
		}
		return i, nil
	case msgp.UintType:
		u, err := dc.ReadUint64()
		if err != nil {
			return 0, err
		}

		// force-cast
		i, ok := castInt64(u)
		if !ok {
			return 0, errors.New("found uint64, overflows int64")
		}
		return i, nil
	default:
		return 0, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// parseUint64 parses an uint64 even if the sent value is an int64;
// this is required because the language used for the encoding library
// may not have unsigned types. An example is early version of Java
// (and so JRuby interpreter) that encodes uint64 as int64:
// http://docs.oracle.com/javase/tutorial/java/nutsandbolts/datatypes.html
func parseUint64(dc *msgp.Reader) (uint64, error) {
	// read the generic representation type without decoding
	t, err := dc.NextType()
	if err != nil {
		return 0, err
	}

	switch t {
	case msgp.UintType:
		u, err := dc.ReadUint64()
		if err != nil {
			return 0, err
		}
		return u, err
	case msgp.IntType:
		i, err := dc.ReadInt64()
		if err != nil {
			return 0, err
		}
		return uint64(i), nil
	default:
		return 0, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// cast to int32 values that are int32 but that are sent in uint32
// over the wire. Set to 0 if they overflow the MaxInt32 size. This
// cast should be used ONLY while decoding int32 values that are
// sent as uint32 to reduce the payload size, otherwise the approach
// is not correct in the general sense.
func castInt32(v uint32) (int32, bool) {
	if v > math.MaxInt32 {
		return 0, false
	}

	return int32(v), true
}

// parseInt32 parses an int32 even if the sent value is an uint32;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt32(dc *msgp.Reader) (int32, error) {
	// read the generic representation type without decoding
	t, err := dc.NextType()
	if err != nil {
		return 0, err
	}

	switch t {
	case msgp.IntType:
		i, err := dc.ReadInt32()
		if err != nil {
			return 0, err
		}
		return i, nil
	case msgp.UintType:
		u, err := dc.ReadUint32()
		if err != nil {
			return 0, err
		}

		// force-cast
		i, ok := castInt32(u)
		if !ok {
			return 0, errors.New("found uint32, overflows int32")
		}
		return i, nil
	default:
		return 0, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// parseStringBytes reads the next type in the msgpack payload and
// converts the BinType or the StrType in a valid string.
func parseStringBytes(bts []byte) (string, []byte, error) {
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var err error
	var i []byte
	switch t {
	case msgp.BinType:
		i, bts, err = msgp.ReadBytesZC(bts)
	case msgp.StrType:
		i, bts, err = msgp.ReadStringZC(bts)
	default:
		return "", bts, msgp.TypeError{Encoded: t, Method: msgp.StrType}
	}
	if err != nil {
		return "", bts, err
	}
	if utf8.Valid(i) {
		return string(i), bts, nil
	}
	return repairUTF8(msgp.UnsafeString(i)), bts, nil
}

// parseFloat64Bytes parses a float64 even if the sent value is an int64 or an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseFloat64Bytes(bts []byte) (float64, []byte, error) {
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var err error
	switch t {
	case msgp.IntType:
		var i int64
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return float64(i), bts, nil
	case msgp.UintType:
		var i uint64
		i, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return float64(i), bts, nil
	case msgp.Float64Type:
		var f float64
		f, bts, err = msgp.ReadFloat64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		return f, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.Float64Type}
	}
}

// parseInt64Bytes parses an int64 even if the sent value is an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt64Bytes(bts []byte) (int64, []byte, error) {
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var i int64
	var u uint64
	var err error
	switch t {
	case msgp.IntType:
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return i, bts, nil
	case msgp.UintType:
		u, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		// force-cast
		i, ok := castInt64(u)
		if !ok {
			return 0, bts, errors.New("found uint64, overflows int64")
		}
		return i, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// parseUint64Bytes parses an uint64 even if the sent value is an int64;
// this is required because the language used for the encoding library
// may not have unsigned types. An example is early version of Java
// (and so JRuby interpreter) that encodes uint64 as int64:
// http://docs.oracle.com/javase/tutorial/java/nutsandbolts/datatypes.html
func parseUint64Bytes(bts []byte) (uint64, []byte, error) {
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var i int64
	var u uint64
	var err error
	switch t {
	case msgp.UintType:
		u, bts, err = msgp.ReadUint64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return u, bts, err
	case msgp.IntType:
		i, bts, err = msgp.ReadInt64Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return uint64(i), bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// parseInt32Bytes parses an int32 even if the sent value is an uint32;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseInt32Bytes(bts []byte) (int32, []byte, error) {
	// read the generic representation type without decoding
	t := msgp.NextType(bts)

	var i int32
	var u uint32
	var err error
	switch t {
	case msgp.IntType:
		i, bts, err = msgp.ReadInt32Bytes(bts)
		if err != nil {
			return 0, bts, err
		}
		return i, bts, nil
	case msgp.UintType:
		u, bts, err = msgp.ReadUint32Bytes(bts)
		if err != nil {
			return 0, bts, err
		}

		// force-cast
		i, ok := castInt32(u)
		if !ok {
			return 0, bts, errors.New("found uint32, overflows int32")
		}
		return i, bts, nil
	default:
		return 0, bts, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}
