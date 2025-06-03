// Read serialize11 variant

package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
)

type transientError struct {
	message string
}

func (e *transientError) Error() string {
	return e.message
}

type byteReader interface {
	io.Reader
	io.ByteReader
}

type streamInterner struct {
	buf []byte

	strings    map[int64]string
	top, limit int64

	tags    map[int64][]string
	topTags int64

	maxSize int64
	maxTags int64

	refName int64
	refTags int64
}

type limitedByteReader struct {
	io.LimitedReader
}

func (lr *limitedByteReader) ReadByte() (byte, error) {
	b := [1]byte{0}
	_, err := io.ReadFull(lr, b[:])
	return b[0], err
}

func newStreamInterner(limit int64) *streamInterner {
	return &streamInterner{
		strings: make(map[int64]string),
		tags:    make(map[int64][]string),
		limit:   limit,
		maxSize: 256,
		maxTags: 100,
	}
}

var errBadRef = &transientError{"reference is larger than the number of items seen so far"}
var errOutOfRange = &transientError{"reference is too large"}
var errTooLong = &transientError{"string is too long"}

func (si *streamInterner) read(r byteReader) (string, error) {
	n, err := binary.ReadVarint(r)
	if err != nil {
		return "", err
	}
	si.refName = n
	if n > 0 {
		if n > si.top {
			return "", errBadRef
		}
		if n > si.limit {
			return "", errOutOfRange
		}
		return si.strings[si.top-n], nil
	}
	if n < -si.maxSize {
		return "", errTooLong
	}

	length := int(-n)
	if length > len(si.buf) {
		si.buf = make([]byte, length)
	}

	_, err = io.ReadFull(r, si.buf[:length])
	if err != nil {
		return "", err
	}

	s := string(si.buf[:length])
	si.strings[si.top] = s
	si.top++

	if si.top > si.limit {
		delete(si.strings, si.top-si.limit-1)
	}

	return s, nil
}

func (si *streamInterner) readTags(r byteReader) ([]string, error) {
	n, err := binary.ReadVarint(r)
	if err != nil {
		return nil, err
	}
	si.refTags = n
	if n > 0 {
		if n > si.topTags {
			return nil, errBadRef
		}
		if n > si.limit {
			return nil, errOutOfRange
		}
		return si.tags[si.topTags-n], nil
	}

	if -n > si.maxTags {
		return nil, errTooLong
	}

	tags := make([]string, int(-n))
	for i := range tags {
		tags[i], err = si.read(r)
		if err != nil {
			return nil, err
		}
	}

	si.tags[si.topTags] = tags
	si.topTags++
	if si.topTags > si.limit {
		delete(si.tags, si.topTags-si.limit-1)
	}

	return tags, nil
}

type reader struct {
	sn *streamInterner
	st *streamInterner

	containerId string
	globalTags  string
	numRecords  int64
	numBytes    uint64

	recordsReader *limitedByteReader
	record        record

	r *bufio.Reader
}

func newReader() *reader {
	return &reader{
		sn: newStreamInterner(256),
		st: newStreamInterner(256),
	}
}

func (r *reader) open(path string) error {
	f, err := os.Open(*flagInput)
	if err != nil {
		return err
	}
	r.r = bufio.NewReader(f)

	return r.readHeader()
}

func (r *reader) readPbTag() (uint64, int, error) {
	tag, err := binary.ReadUvarint(r.r)
	if err != nil {
		return 0, 0, err
	}
	return tag >> 3, int(tag & 7), nil
}

func (r *reader) readPbValue(ty int) (value, error) {
	var val value
	var err error
	switch ty {
	case typeNumber:
		val.value, err = binary.ReadUvarint(r.r)
	case typeBytes:
		len, err := binary.ReadUvarint(r.r)
		if err == nil {
			val.bytes = make([]byte, len)
			_, err = io.ReadFull(r.r, val.bytes)
		}
	}
	return val, err
}

type value struct {
	value uint64
	bytes []byte
}

func (v value) asString() string {
	return string(v.bytes)
}

func (v value) asInt64() int64 {
	return int64(v.value)
}

func (v value) asSint64() int64 {
	return decodeZigzag(v.value)
}

func decodeZigzag(v uint64) int64 {
	if v&1 == 1 {
		return -int64(v>>1) - 1
	} else {
		return int64(v >> 1)
	}
}

var errInvalidDataType = errors.New("invalid type for the metric data field, must be bytes")

const (
	typeNumber = 0
	typeBytes  = 2
)

func (r *reader) readHeader() error {
	const fieldContainerId = 2
	const fieldNumRecords = 5
	const fieldGlobalTags = 6
	const fieldMetricData = 7

	for {
		fid, ty, err := r.readPbTag()
		if err != nil {
			return err
		}

		if fid == fieldMetricData {
			if ty == typeBytes {
				r.numBytes, err = binary.ReadUvarint(r.r)
				if err != nil {
					return err
				}
				r.recordsReader = &limitedByteReader{io.LimitedReader{r.r, int64(r.numBytes)}}
				break
			}
			return errInvalidDataType
		}

		val, err := r.readPbValue(ty)
		if err != nil {
			return err
		}

		switch {
		case fid == fieldContainerId && ty == typeBytes:
			r.containerId = val.asString()
		case fid == fieldNumRecords && ty == typeNumber:
			r.numRecords = val.asInt64()
		case fid == fieldGlobalTags && ty == typeBytes:
			r.globalTags = val.asString()
		default:
			fmt.Printf("unknown field and type combination: %d %d\n", fid, ty)
		}
	}

	return nil
}

var errNotReady = errors.New("must read file header first")

// readRecord reads one record from the stream.
//
// Returns fatal io errors. On invalid or unsupported data sets
// record#error and returns nil.
func (r *reader) readRecord() error {
	r.record.reset()

	if r.recordsReader == nil {
		return errNotReady
	}

	len, err := binary.ReadUvarint(r.recordsReader)
	if err != nil {
		return err
	}

	lr := &limitedByteReader{io.LimitedReader{r.recordsReader, int64(len)}}

	err = r.readFixed(lr)
	if err != nil {
		if _, ok := err.(*transientError); ok {
			r.record.error = err
		} else {
			return err
		}
	}

	_, err = r.r.Discard(int(lr.N))

	return err
}

const (
	gaugeType     = 0
	countType     = 1
	sketchType    = 2
	histogramType = 3
	timingType    = 4
)

func typeName(ty uint64) string {
	switch ty {
	case gaugeType:
		return "gauge"
	case countType:
		return "count"
	case sketchType:
		return "sketch"
	case histogramType:
		return "histogram"
	case timingType:
		return "timing"
	default:
		return "unknown"
	}
}

var errUnknownValueType = &transientError{"unknown metric value type"}
var errUnknownMetricType = &transientError{"unknown metric type"}

var valueTypes = map[uint64]func(br byteReader) (float64, error){
	0x00: readZero,
	0x10: readInt,
	0x20: readFloat,
	0x30: readDouble,
}

func readZero(br byteReader) (float64, error) {
	return 0, nil
}

func readInt(br byteReader) (float64, error) {
	u, err := binary.ReadVarint(br)
	return float64(u), err
}

func readFloat(br byteReader) (float64, error) {
	var buf [4]byte
	_, err := io.ReadFull(br, buf[:])
	if err != nil {
		return 0, err
	}
	var v float32
	_, err = binary.Decode(buf[:], binary.LittleEndian, &v)
	if err != nil {
		return 0, err
	}
	return float64(v), nil
}

func readDouble(br byteReader) (float64, error) {
	var buf [8]byte
	_, err := io.ReadFull(br, buf[:])
	if err != nil {
		return 0, err
	}
	var v float64
	_, err = binary.Decode(buf[:], binary.LittleEndian, &v)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func (r *reader) readFixed(br *limitedByteReader) error {
	var err error
	rec := &r.record

	rec.name, err = r.sn.read(br)
	if err != nil {
		return err
	}

	rec.tags, err = r.st.readTags(br)
	if err != nil {
		return err
	}

	flags, err := binary.ReadUvarint(br)
	if err != nil {
		return err
	}

	rec.ty, rec.valueTy = flags&0xf, flags&0xf0
	valueReader, ok := valueTypes[rec.valueTy]
	if !ok {
		return errUnknownValueType
	}

	switch rec.ty {
	case gaugeType, countType:
		r.record.value, err = valueReader(br)
		if err != nil {
			return err
		}
	default:
		return errUnknownMetricType
	}

	_ = valueReader

	return nil
}

type record struct {
	name  string
	tags  []string
	value float64

	ty, valueTy uint64

	error error
}

func (r *record) reset() {
	*r = record{}
}

var flagInput = flag.String("i", "serialize11.pb", "input file")
var flagProfCpu = flag.String("cpuprofile", "pprof.cpu", "where to write cpu profile to")
var flagVerbose = flag.Bool("v", true, "produce verbose output")

func main() {
	flag.Parse()

	if *flagProfCpu != "" {
		f, err := os.Create(*flagProfCpu)
		if err != nil {
			panic(err)
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			panic(err)
		}
		defer pprof.StopCPUProfile()
	}

	r := newReader()
	if err := r.open(*flagInput); err != nil {
		panic(err)
	}

	fmt.Printf("containerId: %s\n", r.containerId)
	fmt.Printf("global tags: %q\n", r.globalTags)
	fmt.Printf("num records: %d\n", r.numRecords)
	fmt.Printf("data length: %d\n", r.numBytes)

	sum := 0.0
	nerrors := 0
	for {
		err := r.readRecord()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			panic(err)
		}

		rec := &r.record

		if rec.error != nil {
			nerrors++
		} else {
			sum += rec.value
		}

		if *flagVerbose {
			fmt.Println("\n--")
			fmt.Printf("name: (%x) %q\n", r.sn.refName, rec.name)
			fmt.Printf("tags: (%x/%x) %#v\n", r.st.refTags, r.st.refName, rec.tags)
			fmt.Printf("type: (%x) %v, value: %x\n", rec.ty, typeName(rec.ty), rec.valueTy)
			if rec.error != nil {
				fmt.Printf("error: %v\n", rec.error)
			} else {
				fmt.Printf("value: %v\n", rec.value)
			}

			if errors.Is(rec.error, errBadRef) {
				fmt.Printf("%+#v\n", r.sn)
				fmt.Printf("%+#v\n", r.st)
			}
		}
	}

	fmt.Println(sum, nerrors)
}
