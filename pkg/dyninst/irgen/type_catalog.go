// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
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
	ptrSize              uint8
	dwarf                *dwarf.Data
	idAlloc              idAllocator[ir.TypeID]
	typesByDwarfType     map[dwarf.Offset]ir.TypeID
	typesByID            map[ir.TypeID]ir.Type
	typesByGoRuntimeType map[gotype.TypeID]ir.TypeID
}

func newTypeCatalog(
	dwarfData *dwarf.Data,
	ptrSize uint8,
) *typeCatalog {
	return &typeCatalog{
		ptrSize:          ptrSize,
		dwarf:            dwarfData,
		idAlloc:          idAllocator[ir.TypeID]{},
		typesByDwarfType: make(map[dwarf.Offset]ir.TypeID),
		typesByID:        make(map[ir.TypeID]ir.Type),
	}
}

func (c *typeCatalog) addType(offset dwarf.Offset) (ret ir.Type, retErr error) {
	// At most there can be 2 "superficial" typedefs above a real type.  One can
	// happen for structures and subprogram types, and another can happen around
	// generic type parameters nested underneath subprograms (which may then
	// point to the meaningless structure typedef). The depth here includes the
	// two superficial typedefs and the offset of the real type.
	const maxTypedefDepth = 3
	type offsetArray [maxTypedefDepth]dwarf.Offset
	offsets, numOffsets := offsetArray{0: offset}, 1
	defer func() {
		if retErr == nil {
			return
		}
		for ; numOffsets > 0; numOffsets-- {
			retErr = fmt.Errorf("offset 0x%x: %w", offsets[numOffsets-1], retErr)
		}
	}()
	var r *dwarf.Reader
	var entry *dwarf.Entry
	var pt *pointeePlaceholderType
	for {
		offset := offsets[numOffsets-1]
		tid, ok := c.typesByDwarfType[offset]
		if ok {
			t := c.typesByID[tid]
			ppt, ok := t.(*pointeePlaceholderType)
			if !ok {
				return t, nil
			}
			if pt != nil && ppt != pt {
				return nil, errors.New("bug: multiple pointee placeholder types found")
			}
			pt = ppt
		}
		if r == nil {
			r = c.dwarf.Reader()
		}
		r.Seek(offset)
		var err error
		entry, err = r.Next()
		if err != nil {
			return nil, fmt.Errorf("failed to get next entry: %w", err)
		}
		if entry == nil {
			return nil, errors.New("unexpected EOF while reading type")
		}
		if entry.Tag != dwarf.TagTypedef || entry.AttrField(dwAtGoKind) != nil {
			break
		}
		typeOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
		if err != nil {
			return nil, fmt.Errorf("failed to get type for typedef: %w", err)
		}
		if numOffsets++; numOffsets > maxTypedefDepth {
			return nil, errors.New("long typedef chain detected")
		}
		offsets[numOffsets-1] = typeOffset
	}

	var id ir.TypeID
	if pt != nil {
		id = pt.id
	} else {
		id = c.idAlloc.next()
	}
	for _, offset := range offsets[:numOffsets] {
		c.typesByDwarfType[offset] = id
	}
	c.typesByID[id] = &placeHolderType{id: id}
	irType, err := c.buildType(id, entry, r)
	if err != nil {
		return nil, err
	}
	c.typesByID[id] = irType
	return irType, nil
}

func (c *typeCatalog) buildType(
	id ir.TypeID, entry *dwarf.Entry, childReader *dwarf.Reader,
) (ret ir.Type, retErr error) {
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
	// Truncate the size to fit in a uint32. Variables of a size greater than
	// this will be truncated.
	size = min(math.MaxUint32, size)
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
			return nil, errors.New("array type has no children")
		}
	arrayChildren:
		for {
			child, err := childReader.Next()
			if err != nil {
				return nil, fmt.Errorf("failed to get next child: %w", err)
			}
			if child == nil {
				return nil, errors.New(
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
				// Truncate the count to fit in a uint32. Arrays of a count
				// greater than this will be truncated (we would never be able
				// to collect them anyway).
				count = uint32(min(math.MaxUint32, countInt64))
				haveCount = true
			case 0:
				if haveCount {
					break arrayChildren
				}
				return nil, errors.New("unexpected end of array type")
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
			return nil, errors.New("unexpected children for pointer type")
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
		if !hasPointee {
			// unsafe.Pointer is a special case where the type is represented
			// in DWARF as a PointerType, but without a pointee or specified Go kind.
			goAttrs.GoKind = reflect.UnsafePointer
			return &ir.VoidPointerType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
			}, nil
		}

		// Resolve the pointee type.
		var pointee ir.Type
		if id, ok := c.typesByDwarfType[pointeeOffset]; ok {
			pointee = c.typesByID[id]
		} else {
			// We need to check and see if underneath this pointer type there's
			// a typedef that points to an offset for which we already have a
			// type ID.
			r := c.dwarf.Reader()
			r.Seek(pointeeOffset)
			pointeeEntry, err := r.Next()
			if err != nil {
				return nil, err
			}
			if pointeeEntry == nil {
				return nil, errors.New(
					"unexpected EOF while reading pointee type",
				)
			}
			var underlyingTypeOffset dwarf.Offset
			var haveUnderlyingType bool
			if pointeeEntry.Tag == dwarf.TagTypedef &&
				pointeeEntry.AttrField(dwAtGoKind) == nil {
				var err error
				underlyingTypeOffset, err = getAttr[dwarf.Offset](pointeeEntry, dwarf.AttrType)
				if err != nil {
					return nil, err
				}
				if id, ok := c.typesByDwarfType[underlyingTypeOffset]; ok {
					pointee = c.typesByID[id]
					r.Seek(underlyingTypeOffset)
					pointeeEntry, err = r.Next()
					if err != nil {
						return nil, err
					}
					if pointeeEntry == nil {
						return nil, errors.New("unexpected EOF while reading pointee type")
					}
				} else {
					haveUnderlyingType = true
				}
			}
			// If there was no type found, we need to allocate a new placeholder
			// type.
			if pointee == nil {
				attributes, err := getGoTypeAttributes(pointeeEntry)
				if err != nil {
					return nil, err
				}
				pt := &pointeePlaceholderType{
					id:               c.idAlloc.next(),
					offset:           pointeeOffset,
					GoTypeAttributes: attributes,
				}
				pointee = pt
				c.typesByID[pt.id] = pt
				c.typesByDwarfType[pointeeOffset] = pt.id
				if haveUnderlyingType {
					pt.offset = underlyingTypeOffset
					c.typesByDwarfType[underlyingTypeOffset] = pt.id
				}
			}
		}
		return &ir.PointerType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
			Pointee:          pointee,
		}, nil

	case dwarf.TagStructType:
		if !entry.Children {
			return nil, errors.New("structure type has no children")
		}
		fields, err := collectMembers(childReader, c)
		if err != nil {
			return nil, err
		}
		// Note: some of these structure types correspond actually to more
		// Go-specific types like strings, slices, map internals etc. If that's
		// the case, there will be a typedef that carries that information. We
		// handle this later when we finalize the types.
		return &ir.StructureType{
			TypeCommon:       common,
			GoTypeAttributes: goAttrs,
			RawFields:        fields,
		}, nil
	case dwarf.TagTypedef:
		switch goAttrs.GoKind {
		case reflect.Chan:
			return &ir.GoChannelType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
			}, nil
		case reflect.Interface:
			name, byteSize, fields, err := processInterfaceTypedef(c, entry)
			if err != nil {
				return nil, err
			}
			common.ByteSize = uint32(byteSize)
			switch name {
			case "runtime.eface":
				return &ir.GoEmptyInterfaceType{
					TypeCommon:       common,
					GoTypeAttributes: goAttrs,
					RawFields:        fields,
				}, nil
			case "runtime.iface":
				return &ir.GoInterfaceType{
					TypeCommon:       common,
					GoTypeAttributes: goAttrs,
					RawFields:        fields,
				}, nil
			default:
				return nil, fmt.Errorf(
					"unexpected underlying type name for interface: %q", name,
				)
			}
		case reflect.Map:
			underlyingTypeOffset, err := getAttr[dwarf.Offset](
				entry, dwarf.AttrType,
			)
			if err != nil {
				return nil, err
			}
			underlyingType, err := c.addType(underlyingTypeOffset)
			if err != nil {
				return nil, err
			}
			common.ByteSize = underlyingType.GetByteSize()
			headerPtrType, ok := underlyingType.(*ir.PointerType)
			if !ok {
				return nil, fmt.Errorf(
					"underlying type for map is not a pointer type: %T",
					underlyingType,
				)
			}
			pointee := c.typesByID[headerPtrType.Pointee.GetID()]
			if ppt, ok := pointee.(*pointeePlaceholderType); ok {
				pointee, err = c.addType(ppt.offset)
				if err != nil {
					return nil, err
				}
				headerPtrType.Pointee = pointee
			}
			switch pointee.(type) {
			case *ir.StructureType:
			case *ir.GoHMapHeaderType:
			case *ir.GoSwissMapHeaderType:
			default:
				return nil, fmt.Errorf(
					"unexpected underlying type for map header: %T",
					pointee,
				)
			}
			return &ir.GoMapType{
				TypeCommon:       common,
				GoTypeAttributes: goAttrs,
				HeaderType:       pointee,
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

// processInterfaceTypedef processes a typedef that resolves to a runtime
// structure (either runtime.eface or runtime.iface). Each of these structures
// has two pointer fields. One of those is always an unsafe.Pointer (a pointer
// with no pointee type in DWARF). The other is a pointer to a concrete runtime
// type (e.g., runtime._type or runtime.itab). We do not want to pull those
// concrete types into the type graph. Treat both fields as void pointers.
func processInterfaceTypedef(
	c *typeCatalog, entry *dwarf.Entry,
) (name string, byteSize int64, fields []ir.Field, _ error) {
	r := c.dwarf.Reader()
	getUnderlyingEntry := func(entry *dwarf.Entry) (*dwarf.Entry, error) {
		underlyingOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
		if err != nil {
			return nil, err
		}
		r.Seek(underlyingOffset)
		nextEntry, err := r.Next()
		if err != nil {
			return nil, err
		}
		if nextEntry == nil {
			return nil, errors.New(
				"unexpected EOF while reading underlying type",
			)
		}
		return nextEntry, nil
	}
	underlyingEntry := entry
	for underlyingEntry.Tag == dwarf.TagTypedef {
		var err error
		underlyingEntry, err = getUnderlyingEntry(underlyingEntry)
		if err != nil {
			return "", 0, nil, err
		}
		if underlyingEntry.Tag == 0 {
			return "", 0, nil, errors.New("unexpected end of underlying type")
		}
	}
	if underlyingEntry.Tag != dwarf.TagStructType {
		return "", 0, nil, fmt.Errorf(
			"underlying type for interface is not a structure type: %v",
			underlyingEntry.Tag,
		)
	}
	byteSize, ok := underlyingEntry.Val(dwarf.AttrByteSize).(int64)
	if !ok {
		return "", 0, nil, fmt.Errorf(
			"unexpected byte size for interface: %T",
			underlyingEntry.Val(dwarf.AttrByteSize),
		)
	}
	if byteSize > math.MaxUint32 {
		return "", 0, nil, fmt.Errorf(
			"byte size for interface is too large: %d", byteSize,
		)
	}
	fields = make([]ir.Field, 0, 2)
	var voidPointerType ir.Type
	var fieldTypeReader *dwarf.Reader
	// Walk the underlying struct members, capture their offsets and reuse a
	// single void pointer type for all fields.
	for {
		child, err := r.Next()
		if err != nil {
			return "", 0, nil, fmt.Errorf("failed to get next child: %w", err)
		}
		if child == nil {
			return "", 0, nil, errors.New(
				"unexpected EOF while reading underlying type",
			)
		}
		if child.Tag == 0 {
			break
		}
		if child.Tag != dwarf.TagMember {
			continue
		}
		fname, err := getAttr[string](child, dwarf.AttrName)
		if err != nil {
			return "", 0, nil, fmt.Errorf(
				"failed to get name for member %q: %w", fname, err,
			)
		}
		offset, err := getAttr[int64](child, dwarf.AttrDataMemberLoc)
		if err != nil {
			return "", 0, nil, fmt.Errorf(
				"failed to get offset for member %q: %w", fname, err,
			)
		}
		typeOffset, err := getAttr[dwarf.Offset](child, dwarf.AttrType)
		if err != nil {
			return "", 0, nil, fmt.Errorf(
				"failed to get type for member %q: %w", fname, err,
			)
		}
		if fieldTypeReader == nil {
			fieldTypeReader = c.dwarf.Reader()
		}
		fieldTypeReader.Seek(typeOffset)
		fieldTypeEntry, err := fieldTypeReader.Next()
		if err != nil {
			return "", 0, nil, fmt.Errorf(
				"failed to get type for member %q: %w", fname, err,
			)
		}
		if fieldTypeEntry == nil {
			return "", 0, nil, fmt.Errorf(
				"unexpected EOF while reading type for member %q",
				fname,
			)
		}
		// Look for the unsafe.Pointer field. It is represented as a pointer
		// that has no pointee type in DWARF. Reuse that one as the type for
		// all interface fields to avoid expanding runtime types.
		if fieldTypeEntry.Tag == dwarf.TagPointerType &&
			fieldTypeEntry.Val(dwarf.AttrType) == nil &&
			voidPointerType == nil {
			vp, err := c.addType(typeOffset)
			if err != nil {
				return "", 0, nil, fmt.Errorf(
					"failed to add void pointer type for member %q: %w",
					fname, err,
				)
			}
			voidPointerType = vp
		}
		fields = append(fields, ir.Field{
			Name:   fname,
			Offset: uint32(offset),
			// Type set after loop to the void pointer type.
		})
	}
	if voidPointerType == nil {
		return "", 0, nil, fmt.Errorf(
			"failed to find a void pointer field for interface %q", name,
		)
	}
	for i := range fields {
		fields[i].Type = voidPointerType
	}
	name, err := getAttr[string](underlyingEntry, dwarf.AttrName)
	if err != nil {
		return "", 0, nil, fmt.Errorf("failed to get name for interface: %w", err)
	}
	return name, byteSize, fields, nil
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
			return nil, errors.New(
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

type pointeePlaceholderType struct {
	id ir.TypeID
	ir.GoTypeAttributes
	offset dwarf.Offset

	innerPlaceholderType
}

type innerPlaceholderType struct {
	ir.Type
}

func (t *pointeePlaceholderType) GetID() ir.TypeID {
	return t.id
}
