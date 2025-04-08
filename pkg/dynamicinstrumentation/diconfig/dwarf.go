// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package diconfig

import (
	"cmp"
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func getTypeMap(dwarfData *dwarf.Data, targetFunctions map[string]bool) (*ditypes.TypeMap, error) {
	return loadFunctionDefinitions(dwarfData, targetFunctions)
}

type seenTypeCounter struct {
	parameter *ditypes.Parameter
	count     uint8
}

func loadFunctionDefinitions(dwarfData *dwarf.Data, targetFunctions map[string]bool) (*ditypes.TypeMap, error) {
	entryReader := dwarfData.Reader()
	typeReader := dwarfData.Reader()
	readingAFunction := false
	var funcName string

	var result = ditypes.TypeMap{
		Functions: make(map[string][]*ditypes.Parameter),
	}

	var (
		name       string
		isReturn   bool
		typeFields *ditypes.Parameter
		entry      *dwarf.Entry
		err        error
	)
	seenTypes := make(map[string]*seenTypeCounter)

entryLoop:
	for {
		entry, err = entryReader.Next()
		if err == io.EOF || entry == nil {
			break
		}

		if entryIsEmpty(entry) {
			readingAFunction = false
			continue entryLoop
		}

		if entry.Tag == dwarf.TagCompileUnit {
			_, ok := entry.Val(dwarf.AttrName).(string)
			if !ok {
				continue entryLoop
			}
			name = strings.Clone(entry.Val(dwarf.AttrName).(string))
			ranges, err := dwarfData.Ranges(entry)
			if err != nil {
				log.Infof("couldnt retrieve ranges for compile unit %s: %s", name, err)
				continue entryLoop
			}

			cuLineReader, err := dwarfData.LineReader(entry)
			if err != nil {
				log.Errorf("could not get file line reader for compile unit: %v", err)
				continue entryLoop
			}
			var files []*dwarf.LineFile
			if cuLineReader != nil {
				for _, file := range cuLineReader.Files() {
					if file == nil {
						files = append(files, &dwarf.LineFile{Name: "no file", Mtime: 0, Length: 0})
						continue
					}

					files = append(files, &dwarf.LineFile{
						Name:   strings.Clone(file.Name),
						Mtime:  file.Mtime,
						Length: file.Length,
					})
				}
			}

			for i := range ranges {
				result.DeclaredFiles = append(result.DeclaredFiles, &ditypes.DwarfFilesEntry{
					LowPC: ranges[i][0],
					Files: files,
				})
			}
		}

		if entry.Tag == dwarf.TagSubprogram {
			var (
				fn         string
				fileNumber int64
				line       int64
			)
			for _, field := range entry.Field {
				if field.Attr == dwarf.AttrName {
					fn = strings.Clone(field.Val.(string))
				}
				if field.Attr == dwarf.AttrDeclFile {
					fileNumber = field.Val.(int64)
				}
				if field.Attr == dwarf.AttrDeclLine {
					line = field.Val.(int64)
				}
			}

			for _, field := range entry.Field {
				if field.Attr == dwarf.AttrLowpc {
					lowpc := field.Val.(uint64)
					result.FunctionsByPC = append(result.FunctionsByPC, &ditypes.FuncByPCEntry{
						LowPC:      lowpc,
						Fn:         fn,
						FileNumber: fileNumber,
						Line:       line,
					})
				}
			}

			for _, field := range entry.Field {
				if field.Attr == dwarf.AttrName {
					funcName = strings.Clone(field.Val.(string))
					if !targetFunctions[funcName] {
						continue entryLoop
					}
					params := make([]*ditypes.Parameter, 0)
					result.Functions[funcName] = params
					readingAFunction = true
					continue entryLoop
				}
			}
		}

		if !readingAFunction {
			continue
		}

		if entry.Tag != dwarf.TagFormalParameter {
			readingAFunction = false
			continue entryLoop
		}

		// This branch should only be reached if we're currently reading ditypes.Parameters of a function
		// Meaning: This is a formal ditypes.Parameter entry, and readingAFunction = true

		// Go through fields of the entry collecting type, name, size information
		for i := range entry.Field {

			// ditypes.Parameter name
			if entry.Field[i].Attr == dwarf.AttrName {
				name = strings.Clone(entry.Field[i].Val.(string))
			}

			if entry.Field[i].Attr == dwarf.AttrVarParam {
				isReturn = entry.Field[i].Val.(bool)
			}

			// Collect information about the type of this ditypes.Parameter
			if entry.Field[i].Attr == dwarf.AttrType {
				typeReader.Seek(entry.Field[i].Val.(dwarf.Offset))
				typeEntry, err := typeReader.Next()
				if err != nil {
					return nil, err
				}
				typeFields, err = expandTypeData(typeEntry.Offset, dwarfData, seenTypes)
				if err != nil {
					return nil, fmt.Errorf("error while parsing debug information: %w", err)
				}
				clear(seenTypes)
			}
		}

		if typeFields != nil && !isReturn /* we ignore return values for now */ {
			// We've collected information about this ditypes.Parameter, append it to the slice of ditypes.Parameters for this function
			typeFields.Name = name
			result.Functions[funcName] = append(result.Functions[funcName], typeFields)
			typeFields = nil
		}
	}

	// Sort program counter slice for lookup when resolving pcs->functions
	slices.SortFunc(result.FunctionsByPC, func(a, b *ditypes.FuncByPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})
	slices.SortFunc(result.DeclaredFiles, func(a, b *ditypes.DwarfFilesEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	return &result, nil
}

func expandTypeData(offset dwarf.Offset, dwarfData *dwarf.Data, seenTypes map[string]*seenTypeCounter) (*ditypes.Parameter, error) {
	typeReader := dwarfData.Reader()

	typeReader.Seek(offset)
	typeEntry, err := typeReader.Next()
	if err != nil || typeEntry == nil {
		return nil, fmt.Errorf("could not get type entry: %w", err)
	}

	if !entryTypeIsSupported(typeEntry) {
		return resolveUnsupportedEntry(typeEntry), nil
	}

	if typeEntry.Tag == dwarf.TagTypedef {
		typeEntry, err = resolveTypedefToRealType(typeEntry, typeReader)
		if err != nil {
			return nil, err
		}
		if typeEntry == nil {
			return nil, errors.New("could not resolve type entry")
		}
	}

	typeName, typeSize, typeKind := getTypeEntryBasicInfo(typeEntry)
	typeHeader := ditypes.Parameter{
		Type:      typeName,
		TotalSize: typeSize,
		Kind:      typeKind,
	}

	v, typeParsedAlready := seenTypes[typeHeader.Type]
	if typeParsedAlready {
		v.count++
		if v.count > ditypes.MaxReferenceDepth {
			return &ditypes.Parameter{}, nil
		}
	} else {
		seenTypes[typeHeader.Type] = &seenTypeCounter{
			parameter: &typeHeader,
			count:     1,
		}
	}

	if typeEntry.Tag == dwarf.TagStructType || typeKind == uint(reflect.Slice) || typeKind == uint(reflect.String) {
		structFields, err := getStructFields(typeEntry.Offset, dwarfData, seenTypes)
		if err != nil {
			return nil, fmt.Errorf("could not collect fields of struct type of ditypes.Parameter: %w", err)
		}
		typeHeader.ParameterPieces = structFields
	} else if typeEntry.Tag == dwarf.TagArrayType {
		arrayElements, err := getIndividualArrayElements(typeEntry.Offset, dwarfData, seenTypes)
		if err != nil {
			return nil, fmt.Errorf("could not get length of array: %w", err)
		}
		typeHeader.ParameterPieces = arrayElements
	} else if typeEntry.Tag == dwarf.TagPointerType {
		// Get underlying type that the pointer points to
		pointerElements, err := getPointerLayers(typeEntry.Offset, dwarfData, seenTypes)
		if err != nil {
			return nil, fmt.Errorf("could not find pointer type: %w", err)
		}
		typeHeader.ParameterPieces = pointerElements
		// pointers have a unique ID so we only capture the address once when generating
		// location expressions
		typeHeader.ID = randomLabel()
	}

	return &typeHeader, nil
}

func getIndividualArrayElements(offset dwarf.Offset, dwarfData *dwarf.Data, seenTypes map[string]*seenTypeCounter) ([]*ditypes.Parameter, error) {
	savedArrayEntryOffset := offset
	typeReader := dwarfData.Reader()

	// Go to the entry of the array type to get the underlying type information
	typeReader.Seek(offset)
	typeEntry, err := typeReader.Next()
	if err != nil {
		return nil, fmt.Errorf("could not get array type entry: %w", err)
	}

	var (
		elementFields   *ditypes.Parameter
		elementTypeName string
		elementTypeSize int64
		elementTypeKind uint
	)
	underlyingType, err := followType(typeEntry, dwarfData.Reader())
	if err != nil {
		return nil, fmt.Errorf("could not get underlying array type's type entry: %w", err)
	}
	if !entryTypeIsSupported(underlyingType) {
		elementFields = resolveUnsupportedEntry(underlyingType)
		elementTypeName, elementTypeSize, elementTypeKind = getTypeEntryBasicInfo(underlyingType)
	} else {
		arrayElementTypeEntry, err := resolveTypedefToRealType(underlyingType, typeReader)
		if err != nil {
			return nil, err
		}

		elementFields, err = expandTypeData(arrayElementTypeEntry.Offset, dwarfData, seenTypes)
		if err != nil {
			return nil, err
		}

		elementTypeName, elementTypeSize, elementTypeKind = getTypeEntryBasicInfo(arrayElementTypeEntry)
	}

	// Return back to entry of array so we can  go to the subrange entry after the type, which gives
	// us the length of the array
	typeReader.Seek(savedArrayEntryOffset)
	_, err = typeReader.Next()
	if err != nil {
		return nil, fmt.Errorf("could not find array entry: %w", err)
	}
	subrangeEntry, err := typeReader.Next()
	if err != nil {
		return nil, fmt.Errorf("could not get length of array: %w", err)
	}

	var arrayLength int64
	for h := range subrangeEntry.Field {
		if subrangeEntry.Field[h].Attr == dwarf.AttrCount {
			arrayLength = subrangeEntry.Field[h].Val.(int64)
		}
	}

	arrayElements := []*ditypes.Parameter{}
	for h := 0; h < int(arrayLength); h++ {
		newParam := ditypes.Parameter{}
		copyTree(&newParam.ParameterPieces, &elementFields.ParameterPieces)
		newParam.Name = fmt.Sprintf("[%d]%s[%d]", arrayLength, elementTypeName, h)
		newParam.Type = elementTypeName
		newParam.Kind = elementTypeKind
		newParam.TotalSize = elementTypeSize
		arrayElements = append(arrayElements, &newParam)
	}

	return arrayElements, nil
}

func getStructFields(offset dwarf.Offset, dwarfData *dwarf.Data, seenTypes map[string]*seenTypeCounter) ([]*ditypes.Parameter, error) {
	inOrderReader := dwarfData.Reader()
	typeReader := dwarfData.Reader()

	structFields := []*ditypes.Parameter{}
	fieldEntry := &dwarf.Entry{}

	// Start at the entry of the definition of the struct
	inOrderReader.Seek(offset)
	_, err := inOrderReader.Next()
	if err != nil {
		return structFields, err
	}

	// From the struct entry in DWARF, traverse through subsequent DWARF entries
	// which are fields of the struct
	for {
		fieldEntry, err = inOrderReader.Next()
		if err != nil {
			return []*ditypes.Parameter{}, err
		}

		if entryIsEmpty(fieldEntry) || fieldEntry.Tag != dwarf.TagMember {
			break
		}

		newStructField := &ditypes.Parameter{}

		for i := range fieldEntry.Field {

			// Struct Field Name
			if fieldEntry.Field[i].Attr == dwarf.AttrName {
				newStructField.Name = strings.Clone(fieldEntry.Field[i].Val.(string))
			}

			// Struct Field Type
			if fieldEntry.Field[i].Attr == dwarf.AttrType {
				typeReader.Seek(fieldEntry.Field[i].Val.(dwarf.Offset))
				typeEntry, err := typeReader.Next()
				if err != nil {
					return []*ditypes.Parameter{}, err
				}

				if !entryTypeIsSupported(typeEntry) {
					unsupportedType := resolveUnsupportedEntry(typeEntry)
					structFields = append(structFields, unsupportedType)
					continue
				}

				if typeEntry.Tag == dwarf.TagTypedef {
					typeEntry, err = resolveTypedefToRealType(typeEntry, typeReader)
					if err != nil {
						return []*ditypes.Parameter{}, err
					}
				}

				newStructField.Type, newStructField.TotalSize, newStructField.Kind = getTypeEntryBasicInfo(typeEntry)
				if typeEntry.Tag != dwarf.TagBaseType {
					field, err := expandTypeData(typeEntry.Offset, dwarfData, seenTypes)
					if err != nil || field == nil {
						return []*ditypes.Parameter{}, err
					}
					field.Name = newStructField.Name
					structFields = append(structFields, field)
				} else {
					structFields = append(structFields, newStructField)
				}
			}
		}
	}
	return structFields, nil
}

// getPointerLayers is used to populate the underlying type of pointers. The returned slice of parameters
// would contain a single element which represents the entire type tree that the pointer points to.
func getPointerLayers(offset dwarf.Offset, dwarfData *dwarf.Data, seenTypes map[string]*seenTypeCounter) ([]*ditypes.Parameter, error) {
	typeReader := dwarfData.Reader()
	typeReader.Seek(offset)
	pointerEntry, err := typeReader.Next()
	if err != nil {
		return nil, err
	}
	var underlyingType *ditypes.Parameter
	for i := range pointerEntry.Field {

		if pointerEntry.Field[i].Attr == dwarf.AttrType {
			typeReader.Seek(pointerEntry.Field[i].Val.(dwarf.Offset))
			typeEntry, err := typeReader.Next()
			if err != nil {
				return nil, err
			}

			underlyingType, err = expandTypeData(typeEntry.Offset, dwarfData, seenTypes)
			if err != nil {
				return nil, err
			}
		}
	}
	if underlyingType == nil {
		return []*ditypes.Parameter{}, nil
	}
	return []*ditypes.Parameter{underlyingType}, nil
}

// Can use `Children` field, but there's also always a NULL/empty entry at the end of entry trees.
func entryIsEmpty(e *dwarf.Entry) bool {
	return !e.Children &&
		len(e.Field) == 0 &&
		e.Offset == 0 &&
		e.Tag == dwarf.Tag(0)
}

func getTypeEntryBasicInfo(typeEntry *dwarf.Entry) (typeName string, typeSize int64, typeKind uint) {
	if typeEntry.Tag == dwarf.TagPointerType {
		typeSize = 8 // On 64 bit, all pointers are 8 bytes
	}
	for i := range typeEntry.Field {
		if typeEntry.Field[i].Attr == dwarf.AttrName {
			typeName = strings.Clone(typeEntry.Field[i].Val.(string))
		}
		if typeEntry.Field[i].Attr == dwarf.AttrByteSize {
			typeSize = typeEntry.Field[i].Val.(int64)
		}
		if typeEntry.Field[i].Attr == godwarf.AttrGoKind {
			typeKind = uint(typeEntry.Field[i].Val.(int64))
			if typeKind == 0 {
				// Temporary fix for bug: https://github.com/golang/go/issues/64231
				switch typeEntry.Tag {
				case dwarf.TagStructType:
					typeKind = uint(reflect.Struct)
				case dwarf.TagArrayType:
					typeKind = uint(reflect.Array)
				case dwarf.TagPointerType:
					typeKind = uint(reflect.Pointer)
				default:
					log.Info("Unexpected AttrGoKind == 0 for", typeEntry.Tag)
				}
			}
		}
	}
	return
}

func followType(outerType *dwarf.Entry, reader *dwarf.Reader) (*dwarf.Entry, error) {
	for i := range outerType.Field {
		if outerType.Field[i].Attr == dwarf.AttrType {
			reader.Seek(outerType.Field[i].Val.(dwarf.Offset))
			nextType, err := reader.Next()
			if err != nil {
				return nil, fmt.Errorf("error while retrieving underlying type: %w", err)
			}
			return nextType, nil
		}
	}
	return outerType, nil
}

// resolveTypedefToRealType is used to get the underlying type of fields/variables/parameters when
// go packages the type underneath a typdef DWARF entry. The typedef DWARF entry has a 'type' entry
// which points to the actual type, which is what this function 'resolves'.
// Typedef's are used in for structs, pointers, maps, and likely other types.
func resolveTypedefToRealType(outerType *dwarf.Entry, reader *dwarf.Reader) (*dwarf.Entry, error) {

	if outerType.Tag == dwarf.TagTypedef {
		followedType, err := followType(outerType, reader)
		if err != nil {
			return nil, err
		}

		if followedType.Tag == dwarf.TagTypedef {
			return resolveTypedefToRealType(followedType, reader)
		}
		return followedType, nil
	}

	return outerType, nil
}

func copyTree(dst, src *[]*ditypes.Parameter) {
	if dst == nil || src == nil || len(*src) == 0 {
		return
	}
	*dst = make([]*ditypes.Parameter, len(*src))
	copy(*dst, *src)
	for i := range *src {
		// elements can be nil if there was a nil element originally in src
		// that was copied to dst
		if (*dst)[i] == nil || (*src)[i] == nil {
			continue
		}
		copyTree(&((*dst)[i].ParameterPieces), &((*src)[i].ParameterPieces))
	}
}

func kindIsSupported(k reflect.Kind) bool {
	if k == reflect.Map ||
		k == reflect.UnsafePointer ||
		k == reflect.Chan ||
		k == reflect.Float32 ||
		k == reflect.Float64 ||
		k == reflect.Interface {
		return false
	}
	return true
}

func typeIsSupported(t string) bool {
	return t != "unsafe.Pointer"
}

func entryTypeIsSupported(e *dwarf.Entry) bool {
	for f := range e.Field {

		if e.Field[f].Attr == godwarf.AttrGoKind {
			kindOfTypeEntry := reflect.Kind(e.Field[f].Val.(int64))
			if !kindIsSupported(kindOfTypeEntry) {
				return false
			}
		}

		if e.Field[f].Attr == dwarf.AttrName {
			if !typeIsSupported(e.Field[f].Val.(string)) {
				return false
			}
		}
	}
	return true
}

func resolveUnsupportedEntry(e *dwarf.Entry) *ditypes.Parameter {
	var (
		kind uint
		name string
	)
	for f := range e.Field {
		if e.Field[f].Attr == godwarf.AttrGoKind {
			kind = uint(e.Field[f].Val.(int64))
		}
		if e.Field[f].Attr == dwarf.AttrName {
			name = strings.Clone(e.Field[f].Val.(string))
		}
	}
	if name == "unsafe.Pointer" {
		// The DWARF entry for unsafe.Pointer doesn't have a `kind` field
		kind = uint(reflect.UnsafePointer)
	}
	return &ditypes.Parameter{
		Type:             reflect.Kind(kind).String(),
		Kind:             kind,
		NotCaptureReason: ditypes.Unsupported,
		DoNotCapture:     true,
	}
}
