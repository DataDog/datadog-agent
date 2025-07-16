// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

import (
	"reflect"
)

// Type represents a an in-memory representation of a type in the target
// program, or a synthetic type used by the probe to communicate information
// from eBPF to the userspace.
type Type interface {
	// GetID returns the ID of the type.
	GetID() TypeID
	// GetName returns the name of the type.
	GetName() string
	// GetByteSize returns the size of the type in bytes.
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
	_ Type = (*PointerType)(nil)
	_ Type = (*StructureType)(nil)
	_ Type = (*ArrayType)(nil)

	_ Type = (*GoSliceHeaderType)(nil)
	_ Type = (*GoSliceDataType)(nil)
	_ Type = (*GoStringHeaderType)(nil)
	_ Type = (*GoStringDataType)(nil)
	_ Type = (*GoMapType)(nil)
	_ Type = (*GoHMapHeaderType)(nil)
	_ Type = (*GoHMapBucketType)(nil)
	_ Type = (*GoSwissMapHeaderType)(nil)
	_ Type = (*GoSwissMapGroupsType)(nil)
	_ Type = (*GoChannelType)(nil)
	_ Type = (*GoEmptyInterfaceType)(nil)
	_ Type = (*GoInterfaceType)(nil)
	_ Type = (*GoSubroutineType)(nil)

	_ Type = (*EventRootType)(nil)
)

// GetID returns the ID of the type.
func (t *TypeCommon) GetID() TypeID {
	return t.ID
}

// GetName returns the name of the type.
func (t *TypeCommon) GetName() string {
	return t.Name
}

// GetByteSize returns the size of the type in bytes.
func (t *TypeCommon) GetByteSize() uint32 {
	return t.ByteSize
}

// TypeCommon has common fields for all types.
type TypeCommon struct {
	// ID is the ID of the type.
	ID TypeID
	// Name is the name of the type.
	Name string
	// ByteSize is the size of the type in bytes.
	ByteSize uint32
}

// BaseType is a basic type in the target program.
type BaseType struct {
	TypeCommon
	GoTypeAttributes
}

func (t *BaseType) irType() {}

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

	// Fields contains the fields of the structure.
	Fields []Field
}

var _ Type = &StructureType{}

func (t *StructureType) irType() {}

// Field is a field in a structure.
type Field struct {
	// Name is the name of the field.
	Name string
	// Offset in the parent structure.
	Offset uint32
	// Type is the type of the field.
	Type Type
}

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
	UnderlyingStructure *StructureType
}

func (t *GoEmptyInterfaceType) irType() {}

// GoInterfaceType is a type that represents an interface in the target program.
type GoInterfaceType struct {
	TypeCommon
	GoTypeAttributes

	// UnderlyingStructure is the structure that is the underlying type of the
	// runtime.iface.
	UnderlyingStructure *StructureType
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
	// TablePtrSliceType is the slide data type stored conditionally under
	// `dirPtr`.
	TablePtrSliceType *GoSliceDataType
	// GroupType is the slice type stored conditionally under `dirPtr`.
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

// EventRootType is the type of the event output.
type EventRootType struct {
	TypeCommon
	syntheticType

	// Bitset tracking successful expression evaluation (one bit per
	// expression).
	PresenceBitsetSize uint32
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
	// Expression is the logical operations to be evaluated to produce the
	// value of the event.
	Expression Expression
}
