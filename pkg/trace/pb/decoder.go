// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package pb

import (
	"errors"
	io "io"
	"math"
	"sync"

	"github.com/philhofer/fwd"
	"github.com/tinylib/msgp/msgp"
)

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
		return msgp.UnsafeString(i), nil
	case msgp.StrType:
		i, err := dc.ReadString()
		if err != nil {
			return "", err
		}
		return i, nil
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

// DecodeMsgArray implements msgp.Decodable
func (z *Traces) DecodeMsgArray(dc *msgp.Reader) (err error) {
	return z.decodeMsg(dc, (*Span).DecodeMsgArray)
}

// DecodeMsgArray implements msgp.Decodable
func (z *Trace) DecodeMsgArray(dc *msgp.Reader) (err error) {
	return z.decodeMsg(dc, (*Span).DecodeMsgArray)
}

const spanPropertyCount = 12

// DecodeMsgArray implements msgp.Decodable
func (z *Span) DecodeMsgArray(dc *msgp.Reader) (err error) {
	var xsz uint32
	xsz, err = dc.ReadArrayHeader()
	if err != nil {
		return
	}
	if xsz != spanPropertyCount {
		return errors.New("encoded span needs exactly 12 elements in array")
	}
	for i := uint32(0); i < xsz; i++ {
		if dc.IsNil() {
			// empty fields are left at their zero-value
			if err := dc.ReadNil(); err != nil {
				return err
			}
			continue
		}
		switch i {
		case 0:
			// Service
			z.Service, err = parseString(dc)
			if err != nil {
				return
			}
		case 1:
			// Name
			z.Name, err = parseString(dc)
			if err != nil {
				return
			}
		case 2:
			// Resource
			z.Resource, err = parseString(dc)
			if err != nil {
				return
			}
		case 3:
			// TraceID
			z.TraceID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case 4:
			// SpanID
			z.SpanID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case 5:
			// ParentID
			z.ParentID, err = parseUint64(dc)
			if err != nil {
				return
			}
		case 6:
			// Start
			z.Start, err = parseInt64(dc)
			if err != nil {
				return
			}
		case 7:
			// Duration
			z.Duration, err = parseInt64(dc)
			if err != nil {
				return
			}
		case 8:
			// Error
			z.Error, err = parseInt32(dc)
			if err != nil {
				return
			}
		case 9:
			// Meta
			var zwht uint32
			zwht, err = dc.ReadMapHeader()
			if err != nil {
				return
			}
			if z.Meta == nil && zwht > 0 {
				z.Meta = make(map[string]string, zwht)
			} else if len(z.Meta) > 0 {
				for key := range z.Meta {
					delete(z.Meta, key)
				}
			}
			for zwht > 0 {
				zwht--
				var zxvk string
				var zbzg string
				zxvk, err = parseString(dc)
				if err != nil {
					return
				}
				zbzg, err = parseString(dc)
				if err != nil {
					return
				}
				z.Meta[zxvk] = zbzg
			}
		case 10:
			// Metrics
			var zhct uint32
			zhct, err = dc.ReadMapHeader()
			if err != nil {
				return
			}
			if z.Metrics == nil && zhct > 0 {
				z.Metrics = make(map[string]float64, zhct)
			} else if len(z.Metrics) > 0 {
				for key := range z.Metrics {
					delete(z.Metrics, key)
				}
			}
			for zhct > 0 {
				zhct--
				var zbai string
				var zcmr float64
				zbai, err = parseString(dc)
				if err != nil {
					return
				}
				zcmr, err = parseFloat64(dc)
				if err != nil {
					return
				}
				z.Metrics[zbai] = zcmr
			}
		case 11:
			// Type
			z.Type, err = parseString(dc)
			if err != nil {
				return
			}
		}
	}
	return nil
}

var readerPool = sync.Pool{New: func() interface{} { return &msgp.Reader{} }}

// NewMsgpReader returns a *msgp.Reader that
// reads from the provided reader. The
// reader will be buffered.
func NewMsgpReader(r io.Reader) *msgp.Reader {
	p := readerPool.Get().(*msgp.Reader)
	if p.R == nil {
		p.R = fwd.NewReader(r)
	} else {
		p.R.Reset(r)
	}
	return p
}

// FreeMsgpReader marks reader r as done.
func FreeMsgpReader(r *msgp.Reader) { readerPool.Put(r) }
