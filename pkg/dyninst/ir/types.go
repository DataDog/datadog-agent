// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

import (
	"fmt"
	"iter"
	"reflect"
	"slices"
)

// Type represents a an in-memory representation of a type in the target
// program, or a synthetic type used by the probe to communicate information
// from eBPF to the userspace.
type Type interface {
	// GetID returns the ID of the type.
	GetID() TypeID
	// GetName returns the name of the type.
	GetName() string
	// GetDynamicSizeClass returns the class of the dynamic size of the type.
	GetDynamicSizeClass() DynamicSizeClass
	// GetByteSize returns either the size of the type in bytes, for statically
	// sized types, or the size of a single element for dynamically sized types.
	GetByteSize() uint32
	// GetGoRuntimeType returns the runtime type of the type, if it is associated
	// with a Go type.
	GetGoRuntimeType() (uint32, bool)
	// GetGoKind returns the kind of the type, if it is associated with a Go type.
	GetGoKind() (reflect.Kind, bool)

	irType() // marker
}

// GoTypeAttributes is a struct that contains the attributes of a type that is
// associated with a Go type.
type GoTypeAttributes struct {
	// GoRuntimeType is the runtime type of the type, if it is associated with a
	// Go type. It will be zero if the type is not associated with a go type.
	GoRuntimeType uint32
	// GoKind is the kind of the type, if it is associated with a Go type. It
	// will be reflect.Invalid if the type is not associated with a go type.
	GoKind reflect.Kind
}

// GetGoRuntimeType returns the runtime type of the type, if it is associated
// with a Go type.
func (t *GoTypeAttributes) GetGoRuntimeType() (uint32, bool) {
	return t.GoRuntimeType, t.GoRuntimeType != 0
}

// GetGoKind returns the kind of the type, if it is associated with a Go type.
func (t *GoTypeAttributes) GetGoKind() (reflect.Kind, bool) {
	return t.GoKind, t.GoKind != reflect.Invalid
}

var (
	_ Type = (*BaseType)(nil)
	_ Type = (*DurationType)(nil)
	_ Type = (*PointerType)(nil)
	_ Type = (*UnresolvedPointeeType)(nil)
	_ Type = (*StructureType)(nil)
	_ Type = (*ArrayType)(nil)

	_ Type = (*VoidPointerType)(nil)
	_ Type = (*GoSliceHeaderType)(nil)
	_ Type = (*GoSliceDataType)(nil)
	_ Type = (*GoStringHeaderType)(nil)
	_ Type = (*GoStringDataType)(nil)
	_ Type = (*GoMapType)(nil)
	_ Type = (*GoHMapHeaderType)(nil)
	_ Type = (*GoHMapBucketType)(nil)
	_ Type = (*GoSwissMapHeaderType)(nil)
	_ Type = (*GoSwissMapGroupsType)(nil)
	_ Type = (*GoFilteredSliceType)(nil)
	_ Type = (*GoFilteredSliceDataType)(nil)
	_ Type = (*GoFilteredMapType)(nil)
	_ Type = (*GoFilteredMapDataType)(nil)
	_ Type = (*GoTimeType)(nil)
	_ Type = (*GoChannelType)(nil)
	_ Type = (*GoEmptyInterfaceType)(nil)
	_ Type = (*GoInterfaceType)(nil)
	_ Type = (*GoSubroutineType)(nil)

	_ Type = (*EventRootType)(nil)

	_ Type = (*GoContextImplementationType)(nil)
	_ Type = (*DDTraceSpanType)(nil)
)

// GetID returns the ID of the type.
func (t *TypeCommon) GetID() TypeID {
	return t.ID
}

// GetName returns the name of the type.
func (t *TypeCommon) GetName() string {
	return t.Name
}

// GetDynamicSizeClass returns the class of the dynamic size of the type.
func (t *TypeCommon) GetDynamicSizeClass() DynamicSizeClass {
	return t.DynamicSizeClass
}

// GetByteSize returns the size of the type in bytes.
func (t *TypeCommon) GetByteSize() uint32 {
	return t.ByteSize
}

// DynamicSizeClass is the class of the dynamic size of the type.
type DynamicSizeClass uint8

// Note these enum must match the ebpf/types.h:dynamic_size_class enum.
const (
	// StaticSize corresponds to statically sized types.
	StaticSize DynamicSizeClass = iota
	// DynamicSizeSlice corresponds to slices.
	DynamicSizeSlice
	// DynamicSizeString corresponds to strings.
	DynamicSizeString
	// DynamicSizeHashmap corresponds to bucket slice types of hashmaps.
	// These are given extra space due to expected fraction of empty slots.
	DynamicSizeHashmap
	// DynamicSizeFilterDeferred marks per-call-site filter data types whose
	// `enqueue_pc` runs the deferred filter loop. sm_chase_pointer skips the
	// usual serialize step for these types; the enqueue_pc itself emits the
	// per-passing-element data items. Used only by GoFilteredSliceDataType
	// and GoFilteredMapDataType.
	DynamicSizeFilterDeferred
)

// TypeCommon has common fields for all types.
type TypeCommon struct {
	// ID is the ID of the type.
	ID TypeID
	// Name is the name of the type.
	Name string
	// DynamicSize is true if the type is dynamically sized.
	DynamicSizeClass DynamicSizeClass
	// ByteSize is the size of the type in bytes.
	ByteSize uint32
}

// GoContextAttributes describes how a concrete context.Context implementation
// links to its parent and, for context.valueCtx, where the key and value live.
type GoContextAttributes struct {
	ContextOffset int32
	KeyOffset     int32
	ValueOffset   int32
}

const (
	// GoContextNoOffset means the type does not contain that context field.
	GoContextNoOffset int32 = -1
)

// HasChainData reports whether the type is a link in a walkable context chain:
// it carries the layout the BPF chain walk needs to step to the next context,
// namely an embedded parent Context or (for valueCtx) a key/value payload. A
// concrete context.Context implementation with none of these is not a chain
// link (it implements the interface without holding a context of its own, e.g.
// a request type whose methods forward to another context, or one of the
// terminal roots like context.Background); it must be captured as an ordinary
// struct rather than chain-walked.
func (a GoContextAttributes) HasChainData() bool {
	return a.ContextOffset != GoContextNoOffset ||
		a.KeyOffset != GoContextNoOffset ||
		a.ValueOffset != GoContextNoOffset
}

// DDTraceSpanKind identifies the dd-trace-go span layout carried by a type.
type DDTraceSpanKind uint8

const (
	DDTraceSpanNone DDTraceSpanKind = iota
	DDTraceSpanV1
	DDTraceSpanV2
)

// DDTraceAttributes describes a dd-trace-go span layout: where to find each
// of its trace-id / span-id / parent-id / SpanContext fields.
type DDTraceAttributes struct {
	SpanKind                 DDTraceSpanKind
	TraceIDOffset            int32
	SpanIDOffset             int32
	ParentIDOffset           int32
	SpanContextOffset        int32
	SpanContextTraceIDOffset int32
}

// BaseType is a basic type in the target program.
type BaseType struct {
	TypeCommon
	GoTypeAttributes
}

func (t *BaseType) irType() {}

// DurationType is a synthetic 8-byte integer type used by the synthetic
// @duration variable. Its underlying representation is a signed int64 of
// nanoseconds, computed at BPF evaluation time as
// (entry_to_return_duration_ns). It renders in templates and snapshots as
// a float of milliseconds.
type DurationType struct {
	TypeCommon
	syntheticType
}

func (t *DurationType) irType() {}

// ErrDurationNotOnReturn is the user-facing message used when a
// reference to @duration appears on a probe that does not have a paired
// return event. Both irgen (at IR construction time) and decode (when
// the BPF program reports an absent expression status at runtime) need
// to produce the same text, so it lives here next to DurationType.
const ErrDurationNotOnReturn = "@duration is only available at function return"

// TraceContextByteSize is the serialized size of trace_context_t in
// ebpf/types.h. It is the byte size of payload data items of
// TraceContextType.
const TraceContextByteSize uint32 = 40

// TraceContextType is a synthetic 40-byte type used as the type of standalone
// data items emitted by the BPF context-chain walk. The first 40 bytes of the
// payload are interpreted as the trace_context_t layout from ebpf/types.h:
// trace_id_lower, trace_id_upper, span_id, parent_id (8 bytes each),
// followed by a single valid byte and 7 padding bytes. Data items of this
// type are produced by SM_OP_GO_CONTEXT_CHAIN_INIT/HOP at chase time when a
// concrete context.Context implementation is dequeued. The decoder uses them
// (a) to populate the message's top-level dd.trace_id / dd.span_id /
// dd.parent_id fields (first valid one wins) and (b) to render any captured
// context.Context interface field whose data pointer matches the data item's
// address.
type TraceContextType struct {
	TypeCommon
	syntheticType
}

func (t *TraceContextType) irType() {}

// VoidPointerType is a type that represents a pointer to a value of an unknown type.
// unsafe.Pointer is such a type.
type VoidPointerType struct {
	TypeCommon
	GoTypeAttributes
}

func (t *VoidPointerType) irType() {}

// PointerType is a pointer type in the target program.
type PointerType struct {
	TypeCommon
	GoTypeAttributes

	// Pointee is the type that the pointer points to.
	Pointee Type
}

func (t *PointerType) irType() {}

// StructureType is a structure type in the target program.
type StructureType struct {
	TypeCommon
	GoTypeAttributes

	// RawFields contains all the fields of the structure.
	// Use Fields() method to filter out uninteresting fields.
	RawFields []Field
}

var _ Type = &StructureType{}

func (t *StructureType) irType() {}

// Fields returns interesting fields of the structure.
func (t *StructureType) Fields() iter.Seq[Field] {
	return func(yield func(Field) bool) {
		for _, f := range t.RawFields {
			if f.Name == "_" {
				continue
			}
			if !yield(f) {
				return
			}
		}
	}
}

// FieldOffsetByName returns the offset of the field with the given name.
func (t *StructureType) FieldOffsetByName(name string) (uint32, error) {
	field, ok := t.FieldByName(name)
	if !ok {
		return 0, fmt.Errorf("no field %s in struct %s", name, t.Name)
	}
	return field.Offset, nil
}

// FieldByName returns the field with the given name.
func (t *StructureType) FieldByName(name string) (*Field, bool) {
	if idx := slices.IndexFunc(t.RawFields, func(f Field) bool {
		return f.Name == name
	}); idx >= 0 {
		return &t.RawFields[idx], true
	}
	return nil, false
}

// Field is a field in a structure.
type Field struct {
	// Name is the name of the field.
	Name string
	// Offset in the parent structure.
	Offset uint32
	// Type is the type of the field.
	Type Type
}

// GoContextImplementationType wraps a StructureType that is a known concrete
// implementation of context.Context (cancelCtx, valueCtx, timerCtx, …). It
// carries the offsets the BPF chain walk needs to traverse a context chain
// from this struct: the embedded parent Context interface, and (for
// context.valueCtx) the key and value any-fields. The wrapper exists to
// avoid bloating GoTypeAttributes for every IR type with metadata that's
// only meaningful on a tiny number of struct types.
type GoContextImplementationType struct {
	*StructureType
	GoContextAttributes
}

func (t *GoContextImplementationType) irType() {}

// DDTraceSpanType wraps a StructureType that carries a dd-trace-go span
// payload. The wrapper records the span's layout (where the trace ID,
// span ID, parent ID and SpanContext fields sit), so the BPF chain walk
// can extract them when it finds this struct as the value of a context's
// active-span key.
type DDTraceSpanType struct {
	*StructureType
	DDTraceAttributes
}

func (t *DDTraceSpanType) irType() {}

// ArrayType is an array type in the target program.
type ArrayType struct {
	TypeCommon
	GoTypeAttributes

	// Count is the number of elements in the array.
	Count uint32
	// HasCount is true if the array has a count.
	HasCount bool
	// Element is the type of the element in the array.
	Element Type
}

func (t *ArrayType) irType() {}

// GoEmptyInterfaceType is the type of the empty interface (any / interface{}).
type GoEmptyInterfaceType struct {
	TypeCommon
	GoTypeAttributes

	// UnderlyingStructure is the structure that is the underlying type of the
	// runtime.eface.
	RawFields []Field
}

func (t *GoEmptyInterfaceType) irType() {}

// GoInterfaceType is a type that represents an interface in the target program.
type GoInterfaceType struct {
	TypeCommon
	GoTypeAttributes

	// UnderlyingStructure is the structure that is the underlying type of the
	// runtime.iface.
	RawFields []Field
}

func (t *GoInterfaceType) irType() {}

// GoSliceHeaderType is the type of the slice header.
type GoSliceHeaderType struct {
	*StructureType

	// GoSliceDataType is the synthetic type that represents the variable-length array
	// of elements in the slice.
	Data *GoSliceDataType
}

func (GoSliceHeaderType) irType() {}

// GoSliceDataType is a synthetic type that represents the data pointed to by a
// slice header.
type GoSliceDataType struct {
	TypeCommon
	syntheticType

	// Type of the elements in the slice.
	Element Type
}

func (GoSliceDataType) irType() {}

// GoChannelType is a synthetic type that represents a channel.
type GoChannelType struct {
	TypeCommon
	GoTypeAttributes
}

func (GoChannelType) irType() {}

// GoStringHeaderType is the type of the string header.
type GoStringHeaderType struct {
	*StructureType
	Data *GoStringDataType
}

func (GoStringHeaderType) irType() {}

// GoStringDataType is a synthetic type that represents the data pointed
// to by a string header.
type GoStringDataType struct {
	TypeCommon
	syntheticType
}

func (GoStringDataType) irType() {}

// GoMapType is a type that represents a map.
type GoMapType struct {
	TypeCommon
	GoTypeAttributes

	HeaderType Type
}

func (GoMapType) irType() {}

// GoHMapHeaderType is the type of the hash map header.
type GoHMapHeaderType struct {
	*StructureType
	// BucketType is the type of the bucket in the hash map.
	BucketType *GoHMapBucketType
	// BucketsType is the type of the slice of buckets in the hash map.
	BucketsType *GoSliceDataType
}

func (GoHMapHeaderType) irType() {}

// GoHMapBucketType is the type of the bucket in the hash map.
type GoHMapBucketType struct {
	*StructureType
	// KeyType is the type of the key in the hash map.
	KeyType Type
	// ValueType is the type of the value in the hash map.
	ValueType Type
}

func (GoHMapBucketType) irType() {}

// GoSwissMapHeaderType is the type of the header of a SwissMap.
type GoSwissMapHeaderType struct {
	*StructureType

	// TablePtrSliceType is the slice data type stored conditionally under
	// `dirPtr` in the case when dirlen > 0.
	TablePtrSliceType *GoSliceDataType
	// GroupType is the type stored conditionally under `dirPtr` in the case
	// where dirlen == 0.
	GroupType *StructureType
}

func (GoSwissMapHeaderType) irType() {}

// GoSwissMapGroupsType is the type of the groups of a SwissMap.
type GoSwissMapGroupsType struct {
	*StructureType
	// GroupType is the type stored in the slice under `data`.
	GroupType *StructureType
	// GroupSliceType is the type of the slice under `data`.
	GroupSliceType *GoSliceDataType
}

func (GoSwissMapGroupsType) irType() {}

// GoFilteredSliceType is the pointer-shaped, 8-byte handle stored in the
// event-root expression slot for a filter() call whose source is a slice.
// One unique instance is synthesized per filter() call site at irgen time.
// It is NOT a slice header (24 bytes); it is a handle the decoder uses to
// locate the associated per-element data items via their unique type ID.
//
// The wire value is the source slice's data pointer (zero ⇒ nil source).
type GoFilteredSliceType struct {
	TypeCommon
	syntheticType
	// Data is the per-passing-element data-item type whose enqueue_pc
	// implements the deferred filter loop for this call site.
	Data *GoFilteredSliceDataType
}

func (GoFilteredSliceType) irType() {}

// GoFilteredSliceDataType is the per-passing-element data-item type for a
// single filter() call site. Its enqueue_pc bytecode IS the deferred
// filter loop. Because addTypeHandler today switches on type shape only
// and cannot synthesize per-call-site predicate bodies, this type carries
// its own pre-compiled IR op sequence in EnqueueOps. The compiler's
// addTypeHandler switch matches this type and lowers EnqueueOps directly.
type GoFilteredSliceDataType struct {
	TypeCommon
	syntheticType
	// Element is the element type from the source slice's GoSliceDataType.
	Element Type
	// ElemByteSize is the element byte size, duplicated here so the
	// enqueue_pc lowering doesn't need to re-resolve Element.
	ElemByteSize uint32
	// EnqueueOps is the pre-compiled IR op sequence emitted by irgen.
	// It contains only ir.ExpressionOp values:
	//   InitFilterSliceLoopOp
	//   CondLabelOp{Label: BodyLabel}
	//   <predicate body ops>
	//   FilterSliceLoopStepOp{ElementTypeID, ...}
	//   CondLabelOp{Label: EndLabel}
	// The trailing compiler ReturnOp and the per-element CallOp for
	// nested-pointer chasing are introduced by the compiler at lowering
	// time, not stored in EnqueueOps.
	EnqueueOps []ExpressionOp
}

func (GoFilteredSliceDataType) irType() {}

// GoFilteredMapType is the pointer-shaped, 8-byte handle stored in the
// event-root expression slot for a filter() call whose source is a map.
// Analogous to GoFilteredSliceType but for filter results whose source is
// a swiss-table map.
//
// The wire value is the raw map[K]V pointer (zero ⇒ nil source).
type GoFilteredMapType struct {
	TypeCommon
	syntheticType
	// Data is the per-passing-(k,v)-pair data-item type whose enqueue_pc
	// implements the deferred filter loop for this call site.
	Data *GoFilteredMapDataType
}

func (GoFilteredMapType) irType() {}

// GoFilteredMapDataType is the per-passing-(k,v)-pair data-item type for a
// single filter() call site over a map. See GoFilteredSliceDataType for the
// EnqueueOps contract.
type GoFilteredMapDataType struct {
	TypeCommon
	syntheticType
	KeyType   Type
	ValueType Type
	// ValOffsetInPair is the byte offset of the value within the synthetic
	// (key, value) data-item payload. Always (KeyByteSize + 7) & ~7 so the
	// value is 8-byte aligned, matching the @it scratch layout.
	ValOffsetInPair uint32
	// EnqueueOps mirrors GoFilteredSliceDataType.EnqueueOps but contains
	// InitFilterMapLoopOp / FilterMapLoopStepOp instead.
	EnqueueOps []ExpressionOp
}

func (GoFilteredMapDataType) irType() {}

// GoTimeType is a specialized wrapper for the standard library's time.Time
// structure. Decoding time.Time as a generic struct would either leak the
// private wall/ext bit layout or render an opaque blob. By recognizing the
// type during IR generation, the decoder can emit a real RFC3339 timestamp
// and BPF can resolve the *Location pointer to a UTC offset via the
// Location.cacheZone fast path, avoiding any further pointer chasing.
//
// We deliberately only consult the one-element cache (not the full
// tx/zone tables or the extend POSIX string), which covers the common
// case where the program has recently formatted or compared the instant
// in that location; values whose Location has never been exercised
// (e.g. a freshly LoadLocation'd zone, or time.Local before initLocal
// runs) render in UTC instead of their true offset.
//
// All offset/size fields are byte offsets resolved from DWARF. When
// CacheResolved is false the BPF runtime skips the cache lookup and the
// decoder formats in UTC; the remaining cache offset fields are unset in
// that case.
type GoTimeType struct {
	*StructureType

	// WallFieldOffset and ExtFieldOffset are the offsets of the wall and
	// ext fields within the time.Time structure.
	WallFieldOffset uint32
	ExtFieldOffset  uint32
	// LocFieldOffset is the offset of the loc pointer field within the
	// time.Time structure. At runtime BPF overwrites the 8 bytes at this
	// offset with the resolved zone offset (in seconds east of UTC) or
	// the sentinel value GoTimeUnresolvedOffset.
	LocFieldOffset uint32

	// CacheResolved is true when irgen successfully resolved the
	// time.Location field offsets needed for the BPF cache lookup. When
	// false, the BPF runtime writes the unresolved sentinel and the
	// decoder renders in UTC.
	CacheResolved bool
	// CacheStartOffset, CacheEndOffset, CacheZoneOffset are byte offsets
	// within time.Location for the cacheStart, cacheEnd, cacheZone fields.
	CacheStartOffset uint32
	CacheEndOffset   uint32
	CacheZoneOffset  uint32
	// ZoneOffsetFieldOffset is the byte offset of the offset field within
	// the time.zone structure (pointed to by cacheZone).
	ZoneOffsetFieldOffset uint32
	// ZoneOffsetFieldSize is the byte size of the offset field. Go's int
	// is 8 bytes on amd64/arm64 but we record the DWARF size to avoid
	// assumptions.
	ZoneOffsetFieldSize uint32
}

func (GoTimeType) irType() {}

// GoTimeUnresolvedOffset is the sentinel value the BPF runtime writes into
// the loc-pointer slot of a captured time.Time when the timezone offset
// could not be resolved (loc was nil, the Location cache was uninitialized,
// the captured instant fell outside the cache window, or the runtime
// failed to read the cache fields). The decoder maps this value to UTC.
//
// The sentinel is INT64_MIN. Real UTC offsets are bounded by ±14 hours
// (50,400 seconds), so collisions are impossible.
const GoTimeUnresolvedOffset = int64(-1) << 63

// GoSubroutineType is a type that represents a function type in the target
// program.
type GoSubroutineType struct {
	TypeCommon
	GoTypeAttributes
}

func (GoSubroutineType) irType() {}

type syntheticType struct{}

func (syntheticType) GetGoRuntimeType() (uint32, bool) {
	return 0, false
}

func (syntheticType) GetGoKind() (reflect.Kind, bool) {
	return reflect.Invalid, false
}

// DictEntry describes a runtime dictionary entry that will be resolved at
// probe time for generic shape functions. The eBPF reads the dict pointer
// from DictRegister, indexes into it at DictIndex, and writes the resolved
// *runtime._type offset into the event output at Offset.
type DictEntry struct {
	DictIndex    int    // flat index into the dictionary array
	DictRegister uint8  // DWARF register number for the dict pointer
	Offset       uint32 // byte offset in the event output where the resolved type is written
}

// EventRootType is the type of the event output.
type EventRootType struct {
	TypeCommon
	syntheticType

	// EventKind is the kind of the event.
	EventKind EventKind
	// ExprStatusArraySize is the size in bytes of the packed expression
	// status array at the start of the event root data. Each expression
	// occupies ExprStatusBits bits.
	ExprStatusArraySize uint32
	// DictEntries describes runtime dictionary entries to resolve at probe
	// time. Each entry occupies 8 bytes in the event output (after the
	// expression status array, before expressions). Empty for non-generic probes.
	DictEntries []DictEntry
	// Expressions is the list of expressions that are used to evaluate the
	// value of the event.
	Expressions []*RootExpression
}

func (EventRootType) irType() {}

// RootExpression is an expression that is used to evaluate the value of the
// event.
type RootExpression struct {
	// Name is the name of the expression.
	//
	// The name is used in templating to refer to the expression and
	// in the snapshot to name the variable.
	Name string
	// Offset is the offset of the expression in the event output.
	Offset uint32
	// Kind is the kind of the expression.
	Kind RootExpressionKind
	// Expression is the logical operations to be evaluated to produce the
	// value of the event.
	Expression Expression
	// DictIndex is the dictionary index for generic shape type resolution.
	// -1 means no dict resolution needed. When >= 0, the decoder should
	// read the resolved runtime type from the corresponding DictEntry
	// in the EventRootType.
	DictIndex int
	// Redacted is true when the expression resolves to a sensitive value:
	// its source references a redacted variable, member, or string map key.
	// irgen decides this from the parsed expression (the resolved IR keeps
	// only offsets and a display name); the decoder drops the value.
	Redacted bool
}

// RootExpressionKind is the kind of a root expression.
type RootExpressionKind uint8

const (
	_ RootExpressionKind = iota
	// RootExpressionKindArgument corresponds to an argument of the event.
	RootExpressionKindArgument
	// RootExpressionKindLocal corresponds to a local variable of the event.
	RootExpressionKindLocal
	// RootExpressionKindReturn corresponds to a return value of the event.
	RootExpressionKindReturn
	// RootExpressionKindTemplateSegment means that this expression is part of a
	// template segment.
	RootExpressionKindTemplateSegment
	// RootExpressionKindCaptureExpression means that this expression is a
	// capture expression specified by the user.
	RootExpressionKindCaptureExpression
)

func (k RootExpressionKind) String() string {
	switch k {
	case RootExpressionKindArgument:
		return "argument"
	case RootExpressionKindLocal:
		return "local"
	case RootExpressionKindReturn:
		return "return"
	case RootExpressionKindTemplateSegment:
		return "template_segment"
	case RootExpressionKindCaptureExpression:
		return "capture_expression"
	default:
		return fmt.Sprintf("RootExpressionKind(%d)", k)
	}
}

// UnresolvedPointeeType is a placeholder type that represents an unresolved
// pointee type.
type UnresolvedPointeeType struct {
	TypeCommon
	syntheticType
}

func (UnresolvedPointeeType) irType() {}
