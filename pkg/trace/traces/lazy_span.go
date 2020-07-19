package traces

import (
	"fmt"
	"io"
	"math"
	"reflect"
	"unsafe"

	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"
)

const (
	defaultMapInitSize = 16
)

// TODO: This is a fairly naive implementation with a few issues:
//
// 1. Mutations are implemented as append operations. This is extremely fast, but bloats the output
//    payload size. This may or may not be acceptable considering how many mutations are performed,
//    and how good the compression works out, but I expect it to be mostly ok.
// 2. Multiple mutations of the same field will bloat the output even more even though only the
//    last mutation will be visible after deserialization. This is actually easy to fix, we just
//    need to modify the implementation to delay "appending" the mutations until write time.
// 3. It doesn't handle any of the interal meta/metrics maps (yet).
// 4. It stores many of the internal fields as pointer types (string or maps of strings) which will
//    have G.C overhead. Instead, we could store offsets (integers) into the raw []byte where the
//    strings are located and convert them unsafely into string views on demand. This would complicate
//    the implementation slightly, but cause less G.C pressure during the mark phase.
type LazySpan struct {
	raw []byte
	buf []byte
	enc *protoEncoder

	// Top-level fields.
	service   string
	name      string
	resource  string
	traceID   uint64
	spanID    uint64
	parentID  uint64
	start     int64
	duration  int64
	spanError int32
	spanType  string

	meta    map[string]string
	metrics map[string]float64
}

func NewLazySpan(raw []byte) (*LazySpan, error) {
	// TODO: Don't alloc each time?
	buffer := codec.NewBuffer(raw)
	l := &LazySpan{
		raw: raw,
		enc: newProtoEncoder(),
	}
	mapBuf := codec.NewBuffer(nil)
	err := molecule.MessageEach(buffer, func(fieldNum int32, value molecule.Value) (bool, error) {
		switch fieldNum {
		case 1:
			service, err := value.AsStringUnsafe()
			if err != nil {
				return false, err
			}
			l.service = service
		case 2:
			name, err := value.AsStringUnsafe()
			if err != nil {
				return false, err
			}
			l.name = name
		case 3:
			resource, err := value.AsStringUnsafe()
			if err != nil {
				return false, err
			}
			l.resource = resource
		case 4:
			traceID, err := value.AsUint64()
			if err != nil {
				return false, err
			}
			l.traceID = traceID
		case 5:
			spanID, err := value.AsUint64()
			if err != nil {
				return false, err
			}
			l.spanID = spanID
		case 6:
			parentID, err := value.AsUint64()
			if err != nil {
				return false, err
			}
			l.parentID = parentID
		case 7:
			start, err := value.AsInt64()
			if err != nil {
				return false, err
			}
			l.start = start
		case 8:
			duration, err := value.AsInt64()
			if err != nil {
				return false, err
			}
			l.duration = duration
		case 9:
			spanError, err := value.AsInt32()
			if err != nil {
				return false, err
			}
			l.spanError = spanError
		case 10:
			metaBytes, err := value.AsBytesUnsafe()
			if err != nil {
				return false, err
			}

			mapBuf.Reset(metaBytes)

			var (
				key string
				val string
			)
			err = molecule.MessageEach(mapBuf, func(fieldNum int32, value molecule.Value) (bool, error) {
				switch fieldNum {
				case 1:
					str, err := value.AsStringUnsafe()
					if err != nil {
						return false, err
					}
					key = str
				case 2:
					str, err := value.AsStringUnsafe()
					if err != nil {
						return false, err
					}
					val = str
				}
				return true, nil
			})
			if err != nil {
				return false, err
			}
			if l.meta == nil {
				l.meta = make(map[string]string, defaultMapInitSize)
			}
			l.meta[key] = val
		case 11:
			metricBytes, err := value.AsBytesUnsafe()
			if err != nil {
				return false, err
			}

			mapBuf.Reset(metricBytes)

			var (
				key string
				val float64
			)
			err = molecule.MessageEach(mapBuf, func(fieldNum int32, value molecule.Value) (bool, error) {
				switch fieldNum {
				case 1:
					str, err := value.AsStringUnsafe()
					if err != nil {
						return false, err
					}
					key = str
				case 2:
					double, err := value.AsDouble()
					if err != nil {
						return false, err
					}
					val = double
				}
				return true, nil
			})
			if err != nil {
				return false, err
			}
			if l.metrics == nil {
				l.metrics = make(map[string]float64, defaultMapInitSize)
			}
			l.metrics[key] = val
		case 12:
			protoType, err := value.AsStringUnsafe()
			if err != nil {
				return false, err
			}
			l.spanType = protoType
		}

		return true, nil
	})

	if err != nil {
		return nil, fmt.Errorf("NewLazySpan: error parsing span: %v", err)
	}
	return l, nil
}

func (l *LazySpan) TraceID() uint64 {
	return l.traceID
}

func (l *LazySpan) SetTraceID(x uint64) {
	if x == l.traceID {
		return
	}
	l.traceID = x
	l.appendVarint(4, x)
}

func (l *LazySpan) SpanID() uint64 {
	return l.spanID
}

func (l *LazySpan) SetSpanID(x uint64) {
	if x == l.spanID {
		return
	}
	l.spanID = x
	l.appendVarint(5, x)
}

func (l *LazySpan) UnsafeService() string {
	return l.service
}

func (l *LazySpan) SetService(s string) {
	if s == l.service {
		return
	}
	l.service = s
	l.appendString(1, s)
}

func (l *LazySpan) UnsafeName() string {
	return l.name
}

func (l *LazySpan) SetName(s string) {
	if s == l.name {
		return
	}
	l.name = s
	l.appendString(2, s)
}

func (l *LazySpan) UnsafeResource() string {
	return l.resource
}

func (l *LazySpan) SetResource(s string) {
	if s == l.resource {
		return
	}
	l.resource = s
	l.appendString(3, s)
}

func (l *LazySpan) Duration() int64 {
	return l.duration
}

func (l *LazySpan) SetDuration(x int64) {
	if x == l.duration {
		return
	}
	l.duration = x
	l.appendVarint(8, uint64(x))
}

func (l *LazySpan) ParentID() uint64 {
	return l.parentID
}

func (l *LazySpan) SetParentID(x uint64) {
	if x == l.parentID {
		return
	}
	l.parentID = x
	l.appendVarint(6, x)
}

func (l *LazySpan) Start() int64 {
	return l.start
}

func (l *LazySpan) SetStart(x int64) {
	if x == l.start {
		return
	}
	l.start = x
	l.appendVarint(7, uint64(x))
}

func (l *LazySpan) UnsafeType() string {
	return l.spanType
}

func (l *LazySpan) SetType(s string) {
	if s == l.spanType {
		return
	}
	l.spanType = s
	l.appendString(12, s)
}

func (l *LazySpan) Error() int32 {
	return l.spanError
}

func (l *LazySpan) SetError(x int32) {
	if x == l.spanError {
		return
	}
	l.spanError = x
	l.appendVarint(9, uint64(x))
}

func (l *LazySpan) GetMetaUnsafe(s string) (string, bool) {
	v, ok := l.meta[s]
	return v, ok
}

func (l *LazySpan) SetMeta(k, v string) {
	existing, ok := l.meta[k]
	if ok && existing == v {
		return
	}
	l.meta[k] = v
	l.appendMeta(k, v)
}

func (l *LazySpan) ForEachMetaUnsafe(fn MetaIterFunc) {
	for k, v := range l.meta {
		if !fn(k, v) {
			return
		}
	}
}

func (l *LazySpan) GetMetric(s string) (float64, bool) {
	v, ok := l.metrics[s]
	return v, ok
}

func (l *LazySpan) SetMetric(k string, v float64) {
	existing, ok := l.metrics[k]
	if ok && existing == v {
		return
	}
	l.metrics[k] = v
	l.appendMetric(k, v)
}

func (l *LazySpan) ForEachMetricUnsafe(fn MetricIterFunc) {
	for k, v := range l.metrics {
		if !fn(k, v) {
			return
		}
	}
}

func (l *LazySpan) MsgSize() int {
	return len(l.raw)
}

func (l *LazySpan) WriteProto(w io.Writer) error {
	if _, err := w.Write(l.raw); err != nil {
		return fmt.Errorf("LazySpan: WriteProto: error writing span: %v", err)
	}

	return nil
}

func (l *LazySpan) DebugString() string {
	return "TODO"
}

func (l *LazySpan) appendMeta(k, v string) {
	l.buf = l.buf[:0]
	l.enc.reset(l.buf)

	// Map entry key is field 1 with wire type 2 (length-delimited)
	l.enc.encodeTagAndWireType(1, 2)
	l.enc.encodeRawBytes(stringToBytes(k))
	// Map entry value is field 2 with wire type 2 (length-delimited)
	l.enc.encodeTagAndWireType(2, 2)
	l.enc.encodeRawBytes(stringToBytes(v))

	// Capture bytes incase the underlying buf has grown.
	l.buf = l.enc.buf

	l.enc.reset(l.raw)
	l.enc.encodeTagAndWireType(10, 2)
	l.enc.encodeRawBytes(l.buf)
	l.raw = l.enc.buf
}

func (l *LazySpan) appendMetric(k string, v float64) {
	l.buf = l.buf[:0]
	l.enc.reset(l.buf)

	// Map entry key is field 1 with wire type 2 (length-delimited)
	l.enc.encodeTagAndWireType(1, 2)
	l.enc.encodeRawBytes(stringToBytes(k))
	// Map entry key is field 1 with wire type 1 (fixed 64)
	l.enc.encodeTagAndWireType(2, 1)
	l.enc.encodeFixed64(math.Float64bits(v))

	// Capture bytes incase the underlying buf has grown.
	l.buf = l.enc.buf

	l.enc.reset(l.raw)
	l.enc.encodeTagAndWireType(11, 2)
	l.enc.encodeRawBytes(l.buf)
	l.raw = l.enc.buf
}

func (l *LazySpan) appendString(fieldNum int32, s string) {
	l.enc.reset(l.raw)
	l.enc.encodeTagAndWireType(fieldNum, 2)
	l.enc.encodeRawBytes(stringToBytes(s))
	l.raw = l.enc.buf
}

func (l *LazySpan) appendVarint(fieldNum int32, x uint64) {
	l.enc.reset(l.raw)
	l.enc.encodeTagAndWireType(fieldNum, 0)
	l.enc.encodeVarint(x)
	l.raw = l.enc.buf
}

func stringToBytes(str string) []byte {
	hdr := *(*reflect.StringHeader)(unsafe.Pointer(&str))
	return *(*[]byte)(unsafe.Pointer(&reflect.SliceHeader{
		Data: hdr.Data,
		Len:  hdr.Len,
		Cap:  hdr.Len,
	}))
}
