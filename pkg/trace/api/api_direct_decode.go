package api

import (
//	"context"
//	"encoding/json"
//	"expvar"
//	"fmt"
//	"io"
//	"io/ioutil"
//	stdlog "log"
	"math"
//	"mime"
//	"net"
//	"net/http"
//	"net/http/pprof"
//	"os"
//	"runtime"
//	"sort"
//	"strconv"
//	"strings"
//	"sync"
//	"sync/atomic"
//	"time"
//	"bufio"
	"encoding/binary"
	"errors"

	"github.com/tinylib/msgp/msgp"

//	mainconfig "github.com/DataDog/datadog-agent/pkg/config"
//	"github.com/DataDog/datadog-agent/pkg/tagger"
//	"github.com/DataDog/datadog-agent/pkg/tagger/collectors"
//	"github.com/DataDog/datadog-agent/pkg/trace/config"
//	"github.com/DataDog/datadog-agent/pkg/trace/info"
//	"github.com/DataDog/datadog-agent/pkg/trace/logutil"
//	"github.com/DataDog/datadog-agent/pkg/trace/metrics"
//	"github.com/DataDog/datadog-agent/pkg/trace/metrics/timing"
//	"github.com/DataDog/datadog-agent/pkg/trace/osutil"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
//	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
//	"github.com/DataDog/datadog-agent/pkg/trace/watchdog"
//	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// size of every object on the wire,
// plus type information. gives us
// constant-time type information
// for traversing composite objects.
//
var sizes = [256]bytespec{
	mnil:      {size: 1, extra: constsize, typ: msgp.NilType},
	mfalse:    {size: 1, extra: constsize, typ: msgp.BoolType},
	mtrue:     {size: 1, extra: constsize, typ: msgp.BoolType},
	mbin8:     {size: 2, extra: extra8, typ: msgp.BinType},
	mbin16:    {size: 3, extra: extra16, typ: msgp.BinType},
	mbin32:    {size: 5, extra: extra32, typ: msgp.BinType},
	mext8:     {size: 3, extra: extra8, typ: msgp.ExtensionType},
	mext16:    {size: 4, extra: extra16, typ: msgp.ExtensionType},
	mext32:    {size: 6, extra: extra32, typ: msgp.ExtensionType},
	mfloat32:  {size: 5, extra: constsize, typ: msgp.Float32Type},
	mfloat64:  {size: 9, extra: constsize, typ: msgp.Float64Type},
	muint8:    {size: 2, extra: constsize, typ: msgp.UintType},
	muint16:   {size: 3, extra: constsize, typ: msgp.UintType},
	muint32:   {size: 5, extra: constsize, typ: msgp.UintType},
	muint64:   {size: 9, extra: constsize, typ: msgp.UintType},
	mint8:     {size: 2, extra: constsize, typ: msgp.IntType},
	mint16:    {size: 3, extra: constsize, typ: msgp.IntType},
	mint32:    {size: 5, extra: constsize, typ: msgp.IntType},
	mint64:    {size: 9, extra: constsize, typ: msgp.IntType},
	mfixext1:  {size: 3, extra: constsize, typ: msgp.ExtensionType},
	mfixext2:  {size: 4, extra: constsize, typ: msgp.ExtensionType},
	mfixext4:  {size: 6, extra: constsize, typ: msgp.ExtensionType},
	mfixext8:  {size: 10, extra: constsize, typ: msgp.ExtensionType},
	mfixext16: {size: 18, extra: constsize, typ: msgp.ExtensionType},
	mstr8:     {size: 2, extra: extra8, typ: msgp.StrType},
	mstr16:    {size: 3, extra: extra16, typ: msgp.StrType},
	mstr32:    {size: 5, extra: extra32, typ: msgp.StrType},
	marray16:  {size: 3, extra: array16v, typ: msgp.ArrayType},
	marray32:  {size: 5, extra: array32v, typ: msgp.ArrayType},
	mmap16:    {size: 3, extra: map16v, typ: msgp.MapType},
	mmap32:    {size: 5, extra: map32v, typ: msgp.MapType},
}

func init() {
	// set up fixed fields

	// fixint
	for i := mfixint; i < 0x80; i++ {
		sizes[i] = bytespec{size: 1, extra: constsize, typ: msgp.IntType}
	}

	// nfixint
	for i := uint16(mnfixint); i < 0x100; i++ {
		sizes[uint8(i)] = bytespec{size: 1, extra: constsize, typ: msgp.IntType}
	}

	// fixstr gets constsize,
	// since the prefix yields the size
	for i := mfixstr; i < 0xc0; i++ {
		sizes[i] = bytespec{size: 1 + rfixstr(i), extra: constsize, typ: msgp.StrType}
	}

	// fixmap
	for i := mfixmap; i < 0x90; i++ {
		sizes[i] = bytespec{size: 1, extra: varmode(2 * rfixmap(i)), typ: msgp.MapType}
	}

	// fixarray
	for i := mfixarray; i < 0xa0; i++ {
		sizes[i] = bytespec{size: 1, extra: varmode(rfixarray(i)), typ: msgp.ArrayType}
	}
}

// a valid bytespsec has
// non-zero 'size' and
// non-zero 'typ'
type bytespec struct {
	size  uint8   // prefix size information
	extra varmode // extra size information
	typ   msgp.Type    // type
	_     byte    // makes bytespec 4 bytes (yes, this matters)
}

const (
	// Complex64Extension is the extension number used for complex64
	Complex64Extension = 3

	// Complex128Extension is the extension number used for complex128
	Complex128Extension = 4

	// TimeExtension is the extension number used for time.Time
	TimeExtension = 5
)

// size mode
// if positive, # elements for composites
type varmode int8

const (
	constsize varmode = 0  // constant size (size bytes + uint8(varmode) objects)
	extra8            = -1 // has uint8(p[1]) extra bytes
	extra16           = -2 // has be16(p[1:]) extra bytes
	extra32           = -3 // has be32(p[1:]) extra bytes
	map16v            = -4 // use map16
	map32v            = -5 // use map32
	array16v          = -6 // use array16
	array32v          = -7 // use array32
)

func getType(v byte) msgp.Type {
	return sizes[v].typ
}

const last4 = 0x0f
const first4 = 0xf0
const last5 = 0x1f
const first3 = 0xe0
const last7 = 0x7f

func isfixint(b byte) bool {
	return b>>7 == 0
}

func isnfixint(b byte) bool {
	return b&first3 == mnfixint
}

func isfixmap(b byte) bool {
	return b&first4 == mfixmap
}

func isfixarray(b byte) bool {
	return b&first4 == mfixarray
}

func isfixstr(b byte) bool {
	return b&first3 == mfixstr
}

func wfixint(u uint8) byte {
	return u & last7
}

func rfixint(b byte) uint8 {
	return b
}

func wnfixint(i int8) byte {
	return byte(i) | mnfixint
}

func rnfixint(b byte) int8 {
	return int8(b)
}

func rfixmap(b byte) uint8 {
	return b & last4
}

func wfixmap(u uint8) byte {
	return mfixmap | (u & last4)
}

func rfixstr(b byte) uint8 {
	return b & last5
}

func wfixstr(u uint8) byte {
	return (u & last5) | mfixstr
}

func rfixarray(b byte) uint8 {
	return (b & last4)
}

func wfixarray(u uint8) byte {
	return (u & last4) | mfixarray
}

// These are all the byte
// prefixes defined by the
// msgpack standard
const (
	// 0XXXXXXX
	mfixint uint8 = 0x00

	// 111XXXXX
	mnfixint uint8 = 0xe0

	// 1000XXXX
	mfixmap uint8 = 0x80

	// 1001XXXX
	mfixarray uint8 = 0x90

	// 101XXXXX
	mfixstr uint8 = 0xa0

	mnil      uint8 = 0xc0
	mfalse    uint8 = 0xc2
	mtrue     uint8 = 0xc3
	mbin8     uint8 = 0xc4
	mbin16    uint8 = 0xc5
	mbin32    uint8 = 0xc6
	mext8     uint8 = 0xc7
	mext16    uint8 = 0xc8
	mext32    uint8 = 0xc9
	mfloat32  uint8 = 0xca
	mfloat64  uint8 = 0xcb
	muint8    uint8 = 0xcc
	muint16   uint8 = 0xcd
	muint32   uint8 = 0xce
	muint64   uint8 = 0xcf
	mint8     uint8 = 0xd0
	mint16    uint8 = 0xd1
	mint32    uint8 = 0xd2
	mint64    uint8 = 0xd3
	mfixext1  uint8 = 0xd4
	mfixext2  uint8 = 0xd5
	mfixext4  uint8 = 0xd6
	mfixext8  uint8 = 0xd7
	mfixext16 uint8 = 0xd8
	mstr8     uint8 = 0xd9
	mstr16    uint8 = 0xda
	mstr32    uint8 = 0xdb
	marray16  uint8 = 0xdc
	marray32  uint8 = 0xdd
	mmap16    uint8 = 0xde
	mmap32    uint8 = 0xdf
)

func badPrefix(want msgp.Type, lead byte) error {
	t := sizes[lead].typ
	if t == msgp.InvalidType {
		return msgp.InvalidPrefixError(lead)
	}
	return msgp.TypeError{Method: want, Encoded: t}
}

func readArrayHeader(p []byte) (sz uint32, bs []byte, err error) {
	bs = p
	if isfixarray(bs[0]) {
		sz = uint32(rfixarray(bs[0]))
		bs = bs[1:]
		return
	}
	var big = binary.BigEndian
	switch bs[0] {
	case marray16:
		sz = uint32(big.Uint16(bs[1:3]))
		bs = bs[3:]
		return
	case marray32:
		sz = big.Uint32(bs[1:5])
		bs = bs[5:]
		return
	default:
		err = badPrefix(msgp.ArrayType, bs[0])
		return
	}
}

func readMapHeader(p []byte) (sz uint32, bs []byte, err error) {
	bs = p
	if isfixmap(bs[0]) {
		sz = uint32(rfixmap(bs[0]))
		bs = bs[1:]
		return
	}
	var big = binary.BigEndian
	switch bs[0] {
	case mmap16:
		sz = uint32(big.Uint16(bs[1:3]))
		bs = bs[3:]
		return
	case mmap32:
		sz = big.Uint32(bs[1:5])
		bs = bs[5:]
		return
	default:
		err = badPrefix(msgp.MapType, bs[0])
		return
	}
}

func readMapKeyPtr(p []byte) (ptr []byte, bs []byte, err error) {
	bs = p
	var read int
	var big = binary.BigEndian
	if isfixstr(bs[0]) {
		read = int(rfixstr(bs[0]))
		bs = bs[1:]
		goto fill
	}
	switch bs[0] {
	case mstr8, mbin8:
		read = int(bs[1])
		bs = bs[2:]
	case mstr16, mbin16:
		read = int(big.Uint16(bs[1:3]))
		bs = bs[3:]
	case mstr32, mbin32:
		read = int(big.Uint32(bs[1:5]))
		bs = bs[5:]
	default:
		return nil, nil, badPrefix(msgp.StrType, bs[0])
	}
fill:
	if read == 0 {
		return nil, nil, msgp.ErrShortBytes
	}
	return bs[:read], bs[read:], nil
}

func peekExtensionType(p []byte) (int8, error) {
	spec := sizes[p[0]]
	if spec.typ != msgp.ExtensionType {
		return 0, badPrefix(msgp.ExtensionType, p[0])
	}
	if spec.extra == constsize {
		return int8(p[1]), nil
	}
	size := spec.size
	return int8(p[size-1]), nil
}

func nextType(p []byte) (msgp.Type, error) {
	t := getType(p[0])
	if t == msgp.InvalidType {
		return t, msgp.InvalidPrefixError(p[0])
	}
	if t == msgp.ExtensionType {
		v, err := peekExtensionType(p)
		if err != nil {
			return msgp.InvalidType, err
		}
		switch v {
		case msgp.Complex64Extension:
			return msgp.Complex64Type, nil
		case msgp.Complex128Extension:
			return msgp.Complex128Type, nil
		case msgp.TimeExtension:
			return msgp.TimeType, nil
		}
	}
	return t, nil
}

// readBytes reads a MessagePack 'bin' object
// from the reader and returns its value. It may
// use 'scratch' for storage if it is non-nil.
func readBytes(p []byte) (b []byte, bs []byte, err error) {
	bs = p
	lead := bs[0]
	var read int64
	var big = binary.BigEndian
	switch lead {
	case mbin8:
		read = int64(bs[1])
		bs = bs[2:]
	case mbin16:
		read = int64(big.Uint16(bs[1:3]))
		bs = bs[:3]
	case mbin32:
		read = int64(big.Uint32(bs[1:5]))
		bs = bs[1:5]
	default:
		err = badPrefix(msgp.BinType, lead)
		return
	}
	return bs[:read], bs[read:], nil
}

// readString reads a utf-8 string from the reader
func readString(p []byte) (s string, bs []byte, err error) {
	bs = p
	var lead byte
	var read int64
	var big = binary.BigEndian

	lead = bs[0]
	if isfixstr(lead) {
		read = int64(rfixstr(lead))
		bs=bs[1:]
		goto fill
	}

	switch lead {
	case mstr8:
		read = int64(uint8(bs[1]))
		bs = bs[2:]
	case mstr16:
		read = int64(big.Uint16(bs[1:3]))
		bs = bs[3:]
	case mstr32:
		read = int64(big.Uint32(p[1:5]))
		bs = bs[5:]
	default:
		err = badPrefix(msgp.StrType, lead)
		return
	}
fill:
	if read == 0 {
		s, err = "", nil
		return
	}
	return msgp.UnsafeString(bs[:read]), bs[read:], nil
}

func parseString(p []byte) (s string, bs []byte, err error) {
	bs = p
	// read the generic representation type without decoding
	t, err := nextType(bs)
	if err != nil {
		return "", nil, err
	}
	switch t {
	case msgp.BinType:
		i, bs, err := readBytes(bs)
		if err != nil {
			return "", nil, err
		}
		return msgp.UnsafeString(i), bs, nil
	case msgp.StrType:
		i, bs, err := readString(bs)
		if err != nil {
			return "", nil, err
		}
		return i, bs, nil
	default:
		return "", nil, msgp.TypeError{Encoded: t, Method: msgp.StrType}
	}
}

func getMuint8(b []byte) uint8 {
	return uint8(b[1])
}

func getMint8(b []byte) (i int8) {
	return int8(b[1])
}

func getMuint16(b []byte) uint16 {
	return (uint16(b[1]) << 8) | uint16(b[2])
}

func getMint16(b []byte) (i int16) {
	return (int16(b[1]) << 8) | int16(b[2])
}

func getMuint32(b []byte) uint32 {
	return (uint32(b[1]) << 24) | (uint32(b[2]) << 16) | (uint32(b[3]) << 8) | (uint32(b[4]))
}

func getMint32(b []byte) int32 {
	return (int32(b[1]) << 24) | (int32(b[2]) << 16) | (int32(b[3]) << 8) | (int32(b[4]))
}

func getMuint64(b []byte) uint64 {
	return (uint64(b[1]) << 56) | (uint64(b[2]) << 48) |
		(uint64(b[3]) << 40) | (uint64(b[4]) << 32) |
		(uint64(b[5]) << 24) | (uint64(b[6]) << 16) |
		(uint64(b[7]) << 8) | (uint64(b[8]))
}

func getMint64(b []byte) int64 {
	return (int64(b[1]) << 56) | (int64(b[2]) << 48) |
		(int64(b[3]) << 40) | (int64(b[4]) << 32) |
		(int64(b[5]) << 24) | (int64(b[6]) << 16) |
		(int64(b[7]) << 8) | (int64(b[8]))
}

// readUint64 reads a uint64 from the reader
func readUint64(p []byte) (u uint64, bs []byte, err error) {
	bs = p
	var lead byte
	lead = bs[0]
	if isfixint(lead) {
		u = uint64(rfixint(lead))
		bs = bs[1:]
		return
	}
	switch lead {
	case mint8:
		v := int64(getMint8(bs))
		if v < 0 {
			err = msgp.UintBelowZero{Value: v}
			return
		}
		u = uint64(v)
		bs = bs[2:]
		return

	case muint8:
		u = uint64(getMuint8(bs))
		bs = bs[2:]
		return

	case mint16:
		v := int64(getMint16(bs))
		if v < 0 {
			err = msgp.UintBelowZero{Value: v}
			return
		}
		u = uint64(v)
		bs = bs[3:]
		return

	case muint16:
		u = uint64(getMuint16(bs))
		bs = bs[3:]
		return

	case mint32:
		v := int64(getMint32(bs))
		if v < 0 {
			err = msgp.UintBelowZero{Value: v}
			return
		}
		u = uint64(v)
		bs = bs[5:]
		return

	case muint32:
		u = uint64(getMuint32(bs))
		bs = bs[5:]
		return

	case mint64:
		v := int64(getMint64(bs))
		if v < 0 {
			err = msgp.UintBelowZero{Value: v}
			return
		}
		u = uint64(v)
		bs = bs[9:]
		return

	case muint64:
		u = getMuint64(bs)
		bs = bs[9:]
		return

	default:
		if isnfixint(lead) {
			err = msgp.UintBelowZero{Value: int64(rnfixint(lead))}
		} else {
			err = badPrefix(msgp.UintType, lead)
		}
		return

	}
}

// readInt64 reads an int64 from the reader
func readInt64(p []byte) (i int64, bs []byte, err error) {
	bs = p
	lead := bs[0]
	if isfixint(lead) {
		i = int64(rfixint(lead))
		bs = bs[1:]
		return
	} else if isnfixint(lead) {
		i = int64(rnfixint(lead))
		bs = bs[1:]
		return
	}

	switch lead {
	case mint8:
		i = int64(getMint8(bs))
		bs = bs[2:]
		return

	case muint8:
		i = int64(getMuint8(bs))
		bs = bs[2:]
		return

	case mint16:
		i = int64(getMint16(bs))
		bs = bs[3:]
		return

	case muint16:
		i = int64(getMuint16(bs))
		bs = bs[3:]
		return

	case mint32:
		i = int64(getMint32(bs))
		bs = bs[5:]
		return

	case muint32:
		i = int64(getMuint32(bs))
		bs = bs[5:]
		return

	case mint64:
		i = getMint64(bs)
		bs = bs[9:]
		return

	case muint64:
		u := getMuint64(bs)
		if u > math.MaxInt64 {
			err = msgp.UintOverflow{Value: u, FailedBitsize: 64}
			return
		}
		i = int64(u)
		bs = bs[9:]
		return

	default:
		err = badPrefix(msgp.IntType, lead)
		return
	}
}

// parseUint64 parses an uint64 even if the sent value is an int64;
// this is required because the language used for the encoding library
// may not have unsigned types. An example is early version of Java
// (and so JRuby interpreter) that encodes uint64 as int64:
// http://docs.oracle.com/javase/tutorial/java/nutsandbolts/datatypes.html
func parseUint64(p []byte) (uint64, []byte, error) {
	bs := p
	// read the generic representation type without decoding
	t, err := nextType(bs)
	if err != nil {
		return 0, nil, err
	}

	switch t {
	case msgp.UintType:
		u, bs, err := readUint64(bs)
		if err != nil {
			return 0, nil, err
		}
		return u, bs, err
	case msgp.IntType:
		i, bs, err := readInt64(bs)
		if err != nil {
			return 0, nil, err
		}
		return uint64(i), bs, nil
	default:
		return 0, nil, msgp.TypeError{Encoded: t, Method: msgp.IntType}
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
func parseInt64(p []byte) (int64, []byte, error) {
	bs := p
	// read the generic representation type without decoding
	t, err := nextType(bs)
	if err != nil {
		return 0, nil, err
	}

	switch t {
	case msgp.IntType:
		i, bs, err := readInt64(bs)
		if err != nil {
			return 0, nil, err
		}
		return i, bs, nil
	case msgp.UintType:
		u, bs, err := readUint64(bs)
		if err != nil {
			return 0, nil, err
		}

		// force-cast
		i, ok := castInt64(u)
		if !ok {
			return 0, nil, errors.New("found uint64, overflows int64")
		}
		return i, bs, nil
	default:
		return 0, nil, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// readInt32 reads an int32 from the reader
func readInt32(p []byte) (i int32, bs []byte, err error) {
	var in int64
	in, bs, err = readInt64(p)
	if in > math.MaxInt32 || in < math.MinInt32 {
		err = msgp.IntOverflow{Value: in, FailedBitsize: 32}
		return
	}
	i = int32(in)
	return
}

// readUint32 reads a uint32 from the reader
func readUint32(p []byte) (u uint32, bs []byte, err error) {
	var in uint64
	in, bs, err = readUint64(p)
	if in > math.MaxUint32 {
		err = msgp.UintOverflow{Value: in, FailedBitsize: 32}
		return
	}
	u = uint32(in)
	return
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
func parseInt32(p []byte) (int32, []byte, error) {
	bs := p
	// read the generic representation type without decoding
	t, err := nextType(bs)
	if err != nil {
		return 0, nil, err
	}

	switch t {
	case msgp.IntType:
		i, bs, err := readInt32(bs)
		if err != nil {
			return 0, nil, err
		}
		return i, bs, nil
	case msgp.UintType:
		u, bs, err := readUint32(bs)
		if err != nil {
			return 0, nil, err
		}

		// force-cast
		i, ok := castInt32(u)
		if !ok {
			return 0, nil, errors.New("found uint32, overflows int32")
		}
		return i, bs, nil
	default:
		return 0, nil, msgp.TypeError{Encoded: t, Method: msgp.IntType}
	}
}

// returns (skip N bytes, skip M objects, error)
func getSize(b []byte) (uintptr, uintptr, error) {
	l := len(b)
	if l == 0 {
		return 0, 0, msgp.ErrShortBytes
	}
	lead := b[0]
	var big = binary.BigEndian 
	spec := &sizes[lead] // get type information
	size, mode := spec.size, spec.extra
	if size == 0 {
		return 0, 0, msgp.InvalidPrefixError(lead)
	}
	if mode >= 0 { // fixed composites
		return uintptr(size), uintptr(mode), nil
	}
	if l < int(size) {
		return 0, 0, msgp.ErrShortBytes
	}
	switch mode {
	case extra8:
		return uintptr(size) + uintptr(b[1]), 0, nil
	case extra16:
		return uintptr(size) + uintptr(big.Uint16(b[1:])), 0, nil
	case extra32:
		return uintptr(size) + uintptr(big.Uint32(b[1:])), 0, nil
	case map16v:
		return uintptr(size), 2 * uintptr(big.Uint16(b[1:])), nil
	case map32v:
		return uintptr(size), 2 * uintptr(big.Uint32(b[1:])), nil
	case array16v:
		return uintptr(size), uintptr(big.Uint16(b[1:])), nil
	case array32v:
		return uintptr(size), uintptr(big.Uint32(b[1:])), nil
	default:
		// TODO: (knusbaum) This was msgp.fatal, (type msgp.errFatal), but type is not exposed.
		return 0, 0, errors.New("Fatal")
	}
}

// Skip skips over the next object, regardless of
// its type. If it is an array or map, the whole array
// or map will be skipped.
func skip(p []byte) ([]byte, error) {
	var (
		v   uintptr // bytes
		o   uintptr // objects
		err error
		bs []byte
	)

	v, o, err = getSize(bs[:5])
	if err != nil {
		return nil, err
	}
	bs = bs[v:]

	// for maps and slices, skip elements
	for x := uintptr(0); x < o; x++ {
		bs, err = skip(bs)
		if err != nil {
			return nil, err
		}
	}
	return bs, nil
}

// readFloat32 reads a float32 from the reader
func readFloat32(p []byte) (f float32, bs []byte, err error) {
	bs = p
	if bs[0] != mfloat32 {
		err = badPrefix(msgp.Float32Type, bs[0])
		return
	}
	f = math.Float32frombits(getMuint32(bs[:5]))
	bs = bs[5:]
	return
}

// readFloat64 reads a float64 from the reader.
// (If the value on the wire is encoded as a float32,
// it will be up-cast to a float64.)
func readFloat64(p []byte) (f float64, bs []byte, err error) {
	bs = p
	if len(bs) < 9 {
		// we'll allow a coversion from float32 to float64,
		// since we don't lose any precision
		if len(bs) > 0 && bs[0] == mfloat32 {
			ef, bs, err := readFloat32(bs)
			return float64(ef), bs, err
		}
		return
	}
	if p[0] != mfloat64 {
		// see above
		if p[0] == mfloat32 {
			ef, bs, err := readFloat32(bs)
			return float64(ef), bs, err
		}
		err = badPrefix(msgp.Float64Type, bs[0])
		return
	}
	f = math.Float64frombits(getMuint64(bs[:9]))
	bs = bs[9:]
	return
}

// parseFloat64 parses a float64 even if the sent value is an int64 or an uint64;
// this is required because the encoding library could remove bytes from the encoded
// payload to reduce the size, if they're not needed.
func parseFloat64(p []byte) (float64, []byte, error) {
	bs := p
	// read the generic representation type without decoding
	t, err := nextType(bs)
	if err != nil {
		return 0, nil, err
	}

	switch t {
	case msgp.IntType:
		i, bs, err := readInt64(bs)
		if err != nil {
			return 0, nil, err
		}

		return float64(i), bs, nil
	case msgp.UintType:
		i, bs, err := readUint64(bs)
		if err != nil {
			return 0, nil, err
		}

		return float64(i), bs, nil
	case msgp.Float64Type:
		f, bs, err := readFloat64(bs)
		if err != nil {
			return 0, nil, err
		}

		return f, bs, nil
	default:
		return 0, nil, msgp.TypeError{Encoded: t, Method: msgp.Float64Type}
	}
}

func directDecodeSpan(p []byte, z *pb.Span) (bs []byte, err error) {
	var sz uint32
	sz, bs, err = readMapHeader(p)
	if err != nil {
		return nil, err
	}
	for sz > 0 {
		sz--
		p, bs, err = readMapKeyPtr(bs)
		if err != nil {
			return nil, err
		}
		
		switch msgp.UnsafeString(p) {
		case "service":
			if bs[0] == mnil {
				z.Service, err = "", nil
				break
			}
			z.Service, bs, err = parseString(bs)
			if err != nil {
				return
			}
		case "name":
			if bs[0] == mnil {
				z.Name, err = "", nil
				break
			}
			z.Name, bs, err = parseString(bs)
			if err != nil {
				return
			}
		case "resource":
			if bs[0] == mnil {
				z.Resource, err = "", nil
				break
			}
			z.Resource, bs, err = parseString(bs)
			if err != nil {
				return
			}
		case "trace_id":
			if bs[0] == mnil {
				z.TraceID, err = 0, nil
				break
			}
			z.TraceID, bs, err = parseUint64(bs)
			if err != nil {
				return
			}
		case "span_id":
			if bs[0] == mnil {
				z.SpanID, err = 0, nil
				break
			}
			z.SpanID, bs, err = parseUint64(bs)
			if err != nil {
				return
			}
		case "start":
			if bs[0] == mnil {
				z.Start, err = 0, nil
				break
			}
			z.Start, bs, err = parseInt64(bs)
			if err != nil {
				return
			}
		case "duration":
			if bs[0] == mnil {
				z.Duration, err = 0, nil
				break
			}
			z.Duration, bs, err = parseInt64(bs)
			if err != nil {
				return
			}
		case "error":
			if bs[0] == mnil {
				z.Error, err = 0, nil
				break
			}
			z.Error, bs, err = parseInt32(bs)
			if err != nil {
				return
			}
		case "meta":
			if bs[0] == mnil {
				z.Meta, err = nil, nil
				break
			}
			var zwht uint32
			//readMapHeader(p []byte) (sz uint32, bs []byte, err error)
			zwht, bs, err = readMapHeader(bs)
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
				zxvk, bs, err = parseString(bs)
				if err != nil {
					return
				}
				zbzg, bs, err = parseString(bs)
				if err != nil {
					return
				}
				z.Meta[zxvk] = zbzg
			}
		case "metrics":
			if bs[0] == mnil {
				z.Metrics, err = nil, nil
				break
			}

			var zhct uint32
			zhct, bs, err = readMapHeader(bs)
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
				zbai, bs, err = parseString(bs)
				if err != nil {
					return
				}
				zcmr, bs, err = parseFloat64(bs)
				if err != nil {
					return
				}
				z.Metrics[zbai] = zcmr
			}
		case "parent_id":
			if bs[0] == mnil {
				z.ParentID, err = 0, nil
				break
			}

			z.ParentID, bs, err = parseUint64(bs)
			if err != nil {
				return
			}
		case "type":
			if bs[0] == mnil {
				z.Type, err = "", nil
				break
			}

			z.Type, bs, err = parseString(bs)
			if err != nil {
				return
			}
		default:
			bs, err = skip(bs)
			if err != nil {
				return
			}
		}
	}
	return
}

//func readNil(p []byte) (bs []byte, err error) {
//	if p[0] != mnil {
//		return p, badPrefix(msgp.NilType, p[0])
//	}
//	return p[1:], nil
//}

//func directDecodeTraces(req *http.Request, ts *pb.Traces) error {
//	bs := make([]byte, req.ContentLength)
//	_, err := io.ReadFull(req.Body, bs)
//	if err != nil {
//		return err
//	}

func directDecodeTraces(bs []byte, ts *pb.Traces) error {	
	//var sz uint32
	//pb.Traces starts with an array header.
	//fmt.Println("DIRECTDECODETRACES")
	sz, bs, err := readArrayHeader(bs)
	if err != nil {
		return err
	}

	if cap((*ts)) >= int(sz) {
		(*ts) = (*ts)[:sz]
	} else {
		(*ts) = make(pb.Traces, sz)
	}

	for ti := range *ts {
		sz, bs, err = readArrayHeader(bs)
		if err != nil {
			return err
		}
		if cap((*ts)[ti]) >= int(sz) {
			(*ts)[ti] = (*ts)[ti][:sz]
		} else {
			(*ts)[ti] = make(pb.Trace, sz)
		}
		for si := range (*ts)[ti] {
			if bs[0] == mnil {
				bs = bs[1:]
				(*ts)[ti][si] = nil
			} else {
				if (*ts)[ti][si] == nil {
					(*ts)[ti][si] = new(pb.Span)
				}
				bs, err = directDecodeSpan(bs, (*ts)[ti][si])
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}