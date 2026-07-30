// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

// Index-driven per-package type emission.
//
// The Go compiler does not emit each package's type DIEs into that
// package's compile unit; concrete types live in whichever CU the
// compiler first encountered them. emitTypesForPackage uses the prepass
// indexes (typesByPackage + typeInfoByOffset) to enumerate a package's
// types regardless of CU.

package symdb

import (
	"debug/dwarf"
	"fmt"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
)

// dwAtGoKind is the Go-specific DWARF attribute that records a type's
// reflect.Kind. The typedef-chain walk uses it to distinguish real Go
// structs from synthetic TagStructType DIEs the compiler emits for
// slice / string / map / interface header layouts.
const dwAtGoKind dwarf.Attr = 0x2900

// maxTypeUnwrapDepth bounds the typedef/pointer chain walk depth as a
// defense against malformed-DWARF cycles.
const maxTypeUnwrapDepth = 32

// emitTypesForPackage walks every type the prepass indexes attribute to
// pkg and either inserts a new pkg.Types entry or merges into an
// existing one (for types created bare by method-receiver code or by an
// earlier generic instantiation).
func (b *packagesIterator) emitTypesForPackage(pkg *Package) error {
	if b.indexes.typesByPackage == nil || b.indexes.typeInfoByOffset == nil {
		return nil
	}
	reader := b.dwarfData.Reader()
	for offset := range b.indexes.typesByPackage.forPackage(pkg.Name) {
		if err := b.emitTypeForPackage(pkg, offset, reader); err != nil {
			return fmt.Errorf("emitTypesForPackage(%s): %w", pkg.Name, err)
		}
	}
	return nil
}

// emitTypeForPackage handles one (offset → Type) emission. The reader
// is shared across the per-package walk; the function leaves no
// expectation about its position on return.
func (b *packagesIterator) emitTypeForPackage(pkg *Package, offset dwarf.Offset, reader *dwarf.Reader) error {
	rawName, _, ok := b.indexes.typeInfoByOffset.infoAt(offset)
	if !ok {
		// Unreachable: both indexes are populated from the same prepass.
		return fmt.Errorf("type DIE 0x%x in typesByPackage but not in typeInfoByOffset", offset)
	}
	if rawName == "" {
		// typesByPackage gates on parseLinkFuncName which requires a
		// non-empty name, so this is an invariant violation.
		return nil
	}
	pkgName, sym, wasEscaped, err := parseLinkFuncName(rawName)
	if err != nil {
		return fmt.Errorf("parseLinkFuncName %q: %w", rawName, err)
	}
	if pkgName != pkg.Name {
		return fmt.Errorf("type DIE 0x%x: package %q from index does not match name %q", offset, pkg.Name, rawName)
	}
	var unescapedName string
	if wasEscaped {
		unescapedName = pkgName + "." + sym
	} else {
		unescapedName = rawName
	}
	canonicalName := gosymname.CanonicalizeGenerics(unescapedName)

	// Walk the typedef chain on the DWARF reader to find the effective
	// kind and the struct (if any) whose fields populate Type.Fields.
	effectiveKind, fieldsSrcOffset, fieldsSrcEntry, err := b.walkOuterChain(offset, reader)
	if err != nil {
		return fmt.Errorf("walkOuterChain for %q: %w", canonicalName, err)
	}

	// Existing entry: merge-into-bare or merge-across-instantiations.
	if existing, ok := pkg.Types[canonicalName]; ok {
		if effectiveKind == reflect.Struct && fieldsSrcEntry != nil {
			newFields, err := b.structFieldsFromDIE(fieldsSrcOffset, fieldsSrcEntry, reader)
			if err != nil {
				return err
			}
			if len(existing.Fields) == 0 {
				existing.Fields = newFields
			} else {
				mergeStructFields(existing.Fields, newFields)
			}
		}
		return nil
	}

	// typesByPackage's build path already filtered '{', but '<'
	// (compiler-internal forms) and pointer-named types still need
	// rejection here.
	if strings.HasPrefix(unescapedName, "*") {
		return fmt.Errorf("non-pointer type unexpectedly starts with '*': %s", unescapedName)
	}
	if strings.ContainsAny(unescapedName, "{<") {
		return nil
	}

	typ := &Type{
		Name:    canonicalName,
		Fields:  nil,
		Methods: nil,
	}
	if effectiveKind == reflect.Struct && fieldsSrcEntry != nil {
		typ.Fields, err = b.structFieldsFromDIE(fieldsSrcOffset, fieldsSrcEntry, reader)
		if err != nil {
			return err
		}
	}
	pkg.Types[canonicalName] = typ
	return nil
}

// walkOuterChain follows the typedef/pointer chain starting at outer
// and returns the effective Go kind plus, when that kind is
// reflect.Struct, the inner struct DIE whose TagMember children supply
// the type's fields. Pointer DIEs are skipped when classifying the
// kind (their goKind is reflect.Ptr and would mask the underlying
// classification). Field-source is reported only when the inner DIE is
// a real TagStructType, not a synthetic header layout the compiler
// emits for slice/string/map/iface.
func (b *packagesIterator) walkOuterChain(outer dwarf.Offset, reader *dwarf.Reader) (reflect.Kind, dwarf.Offset, *dwarf.Entry, error) {
	kind := reflect.Invalid
	curOffset := outer
	var curEntry *dwarf.Entry
	for step := 0; step < maxTypeUnwrapDepth; step++ {
		reader.Seek(curOffset)
		e, err := reader.Next()
		if err != nil {
			return reflect.Invalid, 0, nil, fmt.Errorf("read DIE 0x%x: %w", curOffset, err)
		}
		if e == nil {
			return reflect.Invalid, 0, nil, fmt.Errorf("no DIE at 0x%x", curOffset)
		}
		curEntry = e
		if kind == reflect.Invalid && e.Tag != dwarf.TagPointerType {
			if k, ok := e.Val(dwAtGoKind).(int64); ok {
				kind = reflect.Kind(k)
			}
		}
		if e.Tag != dwarf.TagPointerType && e.Tag != dwarf.TagTypedef {
			break
		}
		next, ok := e.Val(dwarf.AttrType).(dwarf.Offset)
		if !ok || next == 0 {
			break
		}
		curOffset = next
	}
	if curEntry == nil {
		return reflect.Invalid, 0, nil, fmt.Errorf("walk terminated without DIE for outer 0x%x", outer)
	}
	if kind == reflect.Struct && curEntry.Tag == dwarf.TagStructType {
		return kind, curOffset, curEntry, nil
	}
	return kind, 0, nil, nil
}

// structFieldsFromDIE enumerates a struct DIE's immediate TagMember
// children and resolves each field's type-name through
// typeInfoByOffset. On exit the reader has consumed the struct's child
// block including the closing nil entry.
func (b *packagesIterator) structFieldsFromDIE(structOffset dwarf.Offset, structEntry *dwarf.Entry, reader *dwarf.Reader) ([]Field, error) {
	if !structEntry.Children {
		return nil, nil
	}
	// Re-seek+Next to leave the reader positioned at the parent so the
	// next Next() returns its first child.
	reader.Seek(structOffset)
	if _, err := reader.Next(); err != nil {
		return nil, fmt.Errorf("re-seek struct DIE 0x%x: %w", structOffset, err)
	}
	var fields []Field
	for {
		child, err := reader.Next()
		if err != nil {
			return nil, fmt.Errorf("read struct DIE 0x%x children: %w", structOffset, err)
		}
		if child == nil || child.Tag == 0 {
			break
		}
		if child.Children {
			// TagMember in Go struct DIEs are leaves; defensively skip
			// children of any non-leaf entry to keep the reader aligned.
			reader.SkipChildren()
		}
		if child.Tag != dwarf.TagMember {
			continue
		}
		fieldName, _ := child.Val(dwarf.AttrName).(string)
		fieldTypeOffset, ok := child.Val(dwarf.AttrType).(dwarf.Offset)
		if !ok {
			continue
		}
		fieldTypeName, _, indexed := b.indexes.typeInfoByOffset.infoAt(fieldTypeOffset)
		if !indexed {
			// The field's type DIE wasn't recorded — likely a tag
			// outside the indexed set. Fall back to a one-shot Seek.
			fieldTypeName = b.fieldTypeNameFallback(fieldTypeOffset)
		}
		if strings.Contains(fieldTypeName, "[go.shape.") {
			fieldTypeName = gosymname.CanonicalizeGenerics(fieldTypeName)
		}
		fields = append(fields, Field{
			Name: fieldName,
			Type: fieldTypeName,
		})
	}
	return fields, nil
}

// fieldTypeNameFallback reads just the AttrName of the DIE at offset,
// using a fresh reader because the caller's reader is mid-walk over
// the parent struct's children.
func (b *packagesIterator) fieldTypeNameFallback(offset dwarf.Offset) string {
	r := b.dwarfData.Reader()
	r.Seek(offset)
	e, err := r.Next()
	if err != nil || e == nil {
		return ""
	}
	name, _ := e.Val(dwarf.AttrName).(string)
	return name
}
