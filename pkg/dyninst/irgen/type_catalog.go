// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"debug/dwarf"
	"fmt"
	"math"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

type idAllocator[I interface {
	~uint32
}] struct {
	alloc I
}

func (a *idAllocator[I]) next() I {
	a.alloc++
	return a.alloc
}

type typeCatalog struct {
	maxDynamicTypeSize uint32
	maxHashBucketsSize uint32
	ptrSize            uint8
	dwarf              *dwarf.Data
	idAlloc            idAllocator[ir.TypeID]
	typesByDwarfType   map[dwarf.Offset]ir.TypeID
	typesByID          map[ir.TypeID]ir.Type
}

func newTypeCatalog(
	dwarfData *dwarf.Data,
	ptrSize uint8,
	maxDynamicTypeSize uint32,
	maxHashBucketsSize uint32,
) *typeCatalog {
	return &typeCatalog{
		maxDynamicTypeSize: maxDynamicTypeSize,
		maxHashBucketsSize: maxHashBucketsSize,
		ptrSize:            ptrSize,
		dwarf:              dwarfData,
		idAlloc:            idAllocator[ir.TypeID]{},
		typesByDwarfType:   make(map[dwarf.Offset]ir.TypeID),
		typesByID:          make(map[ir.TypeID]ir.Type),
	}
}

// TODO: Right now this code is going to walk and fill in all reachable types
// from the current entry. This is going to waste memory resources in situations
// where we don't actually need all these types because they won't be reachable
// due to pointer chasing depth limits. This will need to take expressions into
// account. Perhaps the answer there is to leave some sort of placeholders in a
// type marking that we stopped looking and then resume exploration from there
// if we reach a type that needs to be explored further. We'll need some
// bookkeeping on the depth from which a type has been explored.

func (c *typeCatalog) addType(offset dwarf.Offset) (_ ir.Type, retErr error) {
	if id, ok := c.typesByDwarfType[offset]; ok {
		return c.typesByID[id], nil
	}

	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf("offset 0x%x: %w", offset, retErr)
		}
	}()

	reader := c.dwarf.Reader()
	reader.Seek(offset)
	entry, err := reader.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to get next entry: %w", err)
	}
	if entry == nil {
		return nil, fmt.Errorf("unexpected EOF while reading type")
	}
	// We need to figure out whether this is a meaningless typedef as exists
	// for structure types, or if it's a special typedef that go uses for things
	// like channels, interfaces, etc.
	if entry.Tag == dwarf.TagTypedef && entry.AttrField(dwAtGoKind) == nil {
		// We want to now look up the real type and use that instead.
		typeVal := entry.Val(dwarf.AttrType)
		if typeVal == nil {
			// This case is possible in Dwarf, but not clear when it happens.
			// For now, just return an error.
			return nil, fmt.Errorf("missing type for typedef")
		}
		typeOffset, ok := typeVal.(dwarf.Offset)
		if !ok {
			return nil, fmt.Errorf("invalid type for typedef: %T", typeVal)
		}
		underlyingType, err := c.addType(typeOffset)
		if err != nil {
			return nil, err
		}
		c.typesByDwarfType[offset] = underlyingType.GetID()
		return underlyingType, nil
	}

	id := c.idAlloc.next()
	c.typesByDwarfType[offset] = id
	c.typesByID[id] = &placeHolderType{id: id}
	irType, err := c.buildType(id, entry, reader)
	if err != nil {
		return nil, err
	}
	c.typesByID[id] = irType
	return irType, nil
}

func (c *typeCatalog) buildType(
	id ir.TypeID, entry *dwarf.Entry, childReader *dwarf.Reader,
) (_ ir.Type, retErr error) {
	name, err := getAttr[string](entry, dwarf.AttrName)
	if err != nil {
		return nil, fmt.Errorf("failed to get name for array type: %w", err)
	}
	defer func() {
		if retErr != nil {
			retErr = fmt.Errorf("%s type %q: %w", entry.Tag, name, retErr)
		}
	}()
	size, _, err := maybeGetAttr[int64](entry, dwarf.AttrByteSize)
	if err != nil {
		return nil, fmt.Errorf("failed to get size for array type: %w", err)
	}
	if size < 0 {
		return nil, fmt.Errorf("size for type %q is negative: %d", name, size)
	}
	if size > math.MaxUint32 {
		return nil, fmt.Errorf("size for type %q is too large: %d", name, size)
	}
	if size < 0 {
		return nil, fmt.Errorf("size for type %q is negative: %d", name, size)
	}
	common := ir.TypeCommon{
		ID:       id,
		Name:     name,
		ByteSize: uint32(size),
	}
	goAttrs, err := getGoTypeAttributes(entry)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to get go type attributes for array type: %w", err,
		)
	}
	switch entry.Tag {
	case dwarf.TagArrayType:
		var haveCount bool
		var count uint32
		if !entry.Children {
			return nil, fmt.Errorf("array type has no children")
		}
	arrayChildren:
		for {
			child, err := childReader.Next()
			if err != nil {
				return nil, fmt.Errorf("failed to get next child: %w", err)
			}
			if child == nil {
				return nil, fmt.Errorf(
					"unexpected EOF while reading array type",
				)
			}
			switch child.Tag {
			case dwarf.TagSubrangeType:
				countInt64, err := getAttr[int64](child, dwarf.AttrCount)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to get count for subrange type: %w", err,
					)
				}
				if countInt64 < 0 {
					return nil, fmt.Errorf("count for subrange type is negative: %d", countInt64)
				}
				if countInt64 > math.MaxUint32 {
					return nil, fmt.Errorf(
						"count for subrange type is too large: %d", countInt64,
					)
				}
				count = uint32(countInt64)
				haveCount = true
			case 0:
				if haveCount {
					break arrayChildren
				}
				return nil, fmt.Errorf("unexpected end of array type")
			}
		}

		elementOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to get element type for array type: %w", err,
			)
		}
		element, err := c.addType(elementOffset)
		if err != nil {
			return nil, fmt.Errorf("failed to add element type: %w", err)
		}
		return &ir.ArrayType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
			Count:            count,
			HasCount:         true,
			Element:          element,
		}, nil
	case dwarf.TagBaseType:
		encoding, err := getAttr[int64](entry, dwarf.AttrEncoding)
		if err != nil {
			return nil, err
		}
		if err := validateEncoding(
			int(size), encoding, goAttrs.GoKind,
		); err != nil {
			return nil, fmt.Errorf("invalid encoding: %w", err)
		}
		return &ir.BaseType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
		}, nil
	case dwarf.TagPointerType:
		if entry.Children {
			return nil, fmt.Errorf("unexpected children for pointer type")
		}
		if common.ByteSize == 0 {
			common.ByteSize = uint32(c.ptrSize)
		}
		pointeeOffset, hasPointee, err := maybeGetAttr[dwarf.Offset](
			entry, dwarf.AttrType,
		)
		if err != nil {
			return nil, err
		}
		var pointee ir.Type
		if hasPointee {
			pointee, err = c.addType(pointeeOffset)
			if err != nil {
				return nil, err
			}
			return &ir.PointerType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
				Pointee:          pointee,
			}, nil
		}
		return &ir.BaseType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
		}, nil

	case dwarf.TagStructType:
		if !entry.Children {
			return nil, fmt.Errorf("structure type has no children")
		}
		fields, err := collectMembers(childReader, c)
		if err != nil {
			return nil, err
		}
		// TODO: some of these structure types correspond actually to more
		// Go-specific types like strings, slices, map internals etc.
		return &ir.StructureType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
			Fields:           fields,
		}, nil
	case dwarf.TagTypedef:
		getUnderlyingType := func() (ir.Type, error) {
			underlyingTypeOffset, err := getAttr[dwarf.Offset](
				entry, dwarf.AttrType,
			)
			if err != nil {
				return nil, err
			}
			return c.addType(underlyingTypeOffset)
		}
		switch goAttrs.GoKind {
		case reflect.Chan:
			return &ir.GoChannelType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
			}, nil
		case reflect.Interface:
			underlyingType, err := getUnderlyingType()
			if err != nil {
				return nil, err
			}
			underlyingStructure, ok := underlyingType.(*ir.StructureType)
			if !ok {
				return nil, fmt.Errorf(
					"underlying type for interface is not a structure type: %T",
					underlyingType,
				)
			}
			common.ByteSize = underlyingStructure.GetByteSize()
			switch name := underlyingStructure.GetName(); name {
			case "runtime.eface":
				return &ir.GoEmptyInterfaceType{
					TypeCommon:          common,
					GoTypeAttributes:    goAttrs,
					UnderlyingStructure: underlyingStructure,
				}, nil
			case "runtime.iface":
				return &ir.GoInterfaceType{
					TypeCommon:          common,
					GoTypeAttributes:    goAttrs,
					UnderlyingStructure: underlyingStructure,
				}, nil
			default:
				return nil, fmt.Errorf(
					"unexpected underlying type name for interface: %q", name,
				)
			}
		case reflect.Map:
			underlyingType, err := getUnderlyingType()
			if err != nil {
				return nil, err
			}
			headerPtrType, ok := underlyingType.(*ir.PointerType)
			if !ok {
				return nil, fmt.Errorf(
					"underlying type for map is not a pointer type: %T",
					underlyingType,
				)
			}
			headerType, ok := headerPtrType.Pointee.(*ir.StructureType)
			if !ok {
				return nil, fmt.Errorf(
					"underlying type for map is not a structure type: %T",
					headerPtrType.Pointee,
				)
			}
			return &ir.GoMapType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
				HeaderType:       headerType,
			}, nil
		default:
			return nil, fmt.Errorf(
				"typedef for kind %v not implemented", goAttrs.GoKind,
			)
		}
	case dwarf.TagSubroutineType:
		// TODO: We could collect up and model the parameters here.
		if entry.Children {
			childReader.SkipChildren()
		}
		if goAttrs.GoKind != reflect.Func {
			return nil, fmt.Errorf(
				"subroutine type has kind %v, expected %v",
				goAttrs.GoKind,
				reflect.Func,
			)
		}
		return &ir.GoSubroutineType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
		}, nil
	default:
		return nil, fmt.Errorf("unexpected tag for type: %s", entry.Tag)
	}
}

func collectMembers(childReader *dwarf.Reader, c *typeCatalog) ([]ir.Field, error) {
	fields := []ir.Field{}
structChildren:
	for {
		child, err := childReader.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to get next child: %w", err)
		}
		if child == nil {
			return nil, fmt.Errorf(
				"unexpected EOF while reading structure type",
			)
		}
		switch child.Tag {
		case dwarf.TagMember:
			name, err := getAttr[string](child, dwarf.AttrName)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to get name for member %q: %w", name, err,
				)
			}
			offset, err := getAttr[int64](child, dwarf.AttrDataMemberLoc)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to get offset for member %q: %w", name, err,
				)
			}
			typeOffset, err := getAttr[dwarf.Offset](child, dwarf.AttrType)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to get type for member %q: %w", name, err,
				)
			}
			fieldType, err := c.addType(typeOffset)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to add type for member %q: %w", name, err,
				)
			}
			fields = append(fields, ir.Field{
				Name:   name,
				Offset: uint32(offset),
				Type:   fieldType,
			})
		case 0:
			break structChildren
		default:
			return nil, fmt.Errorf(
				"unexpected tag while collecting members at offset 0x%x: %s",
				child.Offset, child.Tag,
			)
		}
	}
	return fields, nil
}

func validateEncoding(byteSize int, encoding int64, goKind reflect.Kind) error {
	// NB: This function assumes that we're only targeting 64-bit systems.
	type sizeEncoding struct {
		size     int
		encoding int64
		kind     reflect.Kind
	}
	type se = sizeEncoding
	enc := se{size: byteSize, encoding: encoding, kind: goKind}
	switch enc {
	case se{1, dwAteBoolean, reflect.Bool}:
	case se{1, dwAteUnsigned, reflect.Uint8}:
	case se{1, dwAteSigned, reflect.Int8}:
	case se{2, dwAteUnsigned, reflect.Uint16}:
	case se{2, dwAteSigned, reflect.Int16}:
	case se{4, dwAteUnsigned, reflect.Uint32}:
	case se{4, dwAteSigned, reflect.Int32}:
	case se{4, dwAteFloat, reflect.Float32}:
	case se{8, dwAteUnsigned, reflect.Uint64}:
	case se{8, dwAteUnsigned, reflect.Uintptr}:
	case se{8, dwAteUnsigned, reflect.Uint}:
	case se{8, dwAteSigned, reflect.Int64}:
	case se{8, dwAteSigned, reflect.Int}:
	case se{8, dwAteFloat, reflect.Float64}:
	case se{8, dwAteComplexFloat, reflect.Complex64}:
	case se{16, dwAteComplexFloat, reflect.Complex128}:
	default:
		return fmt.Errorf(
			"unexpected kind (%v) for size and encoding (%v, %v)",
			enc.kind, enc.size, enc.encoding,
		)
	}
	return nil
}

func getGoTypeAttributes(entry *dwarf.Entry) (ir.GoTypeAttributes, error) {
	var goAttrs ir.GoTypeAttributes
	goRuntimeType, _, err := maybeGetAttr[uint64](entry, dwAtGoRuntimeType)
	if err != nil {
		return ir.GoTypeAttributes{}, err
	}
	goAttrs.GoRuntimeType = uint32(goRuntimeType)
	goKind, _, err := maybeGetAttr[int64](entry, dwAtGoKind)
	if err != nil {
		return ir.GoTypeAttributes{}, err
	}
	goAttrs.GoKind = reflect.Kind(goKind)
	return goAttrs, nil
}

var _ ir.Type = &placeHolderType{}

type placeHolderType struct {
	ir.Type
	id ir.TypeID
}

func (t *placeHolderType) GetID() ir.TypeID {
	return t.id
}
