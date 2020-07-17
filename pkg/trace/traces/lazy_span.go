package traces

import (
	"fmt"
	"io"

	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"
)

// TODO: Implement me with molecule.
type LazySpan struct {
	raw []byte

	service string
	name    string

	resource  string
	traceID   uint64
	spanID    uint64
	parentID  uint64
	start     int64
	duration  int64
	spanError int32
	meta      map[string]string
	metrics   map[string]float64
	spanType  string
}

// Service  string             `protobuf:"bytes,1,opt,name=service,proto3" json:"service" msg:"service"`
// 	Name     string             `protobuf:"bytes,2,opt,name=name,proto3" json:"name" msg:"name"`
// 	Resource string             `protobuf:"bytes,3,opt,name=resource,proto3" json:"resource" msg:"resource"`
// 	TraceID  uint64             `protobuf:"varint,4,opt,name=traceID,proto3" json:"trace_id" msg:"trace_id"`
// 	SpanID   uint64             `protobuf:"varint,5,opt,name=spanID,proto3" json:"span_id" msg:"span_id"`
// 	ParentID uint64             `protobuf:"varint,6,opt,name=parentID,proto3" json:"parent_id" msg:"parent_id"`
// 	Start    int64              `protobuf:"varint,7,opt,name=start,proto3" json:"start" msg:"start"`
// 	Duration int64              `protobuf:"varint,8,opt,name=duration,proto3" json:"duration" msg:"duration"`
// 	Error    int32              `protobuf:"varint,9,opt,name=error,proto3" json:"error" msg:"error"`
// 	Meta     map[string]string  `protobuf:"bytes,10,rep,name=meta" json:"meta" msg:"meta" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
// 	Metrics  map[string]float64 `protobuf:"bytes,11,rep,name=metrics" json:"metrics" msg:"metrics" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"fixed64,2,opt,name=value,proto3"`
// 	Type     string             `protobuf:"bytes,12,opt,name=type,proto3" json:"type" msg:"type"`
// }

func NewLazySpan(raw []byte) (*LazySpan, error) {
	// TODO: Don't alloc each time?
	buffer := codec.NewBuffer(raw)
	// buffer.
	l := &LazySpan{
		raw: raw,
	}
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
			// TODO: Handle meta
		case 11:
			// TODO: Handle metrics
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
	l.traceID = x
}

func (l *LazySpan) SpanID() uint64 {
	return l.spanID
}

func (l *LazySpan) SetSpanID(x uint64) {
	l.spanID = x
}

func (l *LazySpan) UnsafeService() string {
	// This operation is actually safe in this implementation, but callers should behave like its not.
	return l.service
}

func (l *LazySpan) SetService(s string) {
	l.service = s
}

func (l *LazySpan) UnsafeName() string {
	// This operation is actually safe in this implementation, but callers should behave like its not.
	return l.name
}

func (l *LazySpan) SetName(s string) {
	l.name = s
}

func (l *LazySpan) UnsafeResource() string {
	// This operation is actually safe in this implementation, but callers should behave like its not.
	return l.resource
}

func (l *LazySpan) SetResource(s string) {
	l.resource = s
}

func (l *LazySpan) Duration() int64 {
	return l.duration
}

func (l *LazySpan) SetDuration(d int64) {
	l.duration = d
}

func (l *LazySpan) ParentID() uint64 {
	return l.parentID
}

func (l *LazySpan) SetParentID(id uint64) {
	l.parentID = id
}

func (l *LazySpan) Start() int64 {
	return l.start
}

func (l *LazySpan) SetStart(d int64) {
	l.start = d
}

func (l *LazySpan) UnsafeType() string {
	// This operation is actually safe in this implementation, but callers should behave like its not.
	return l.spanType
}

func (l *LazySpan) SetType(s string) {
	l.spanType = s
}

func (l *LazySpan) Error() int32 {
	// This operation is actually safe in this implementation, but callers should behave like its not.
	return l.spanError
}

func (l *LazySpan) SetError(x int32) {
	l.spanError = x
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
