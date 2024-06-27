package diconfig

import (
	"cmp"
	"debug/dwarf"
	"debug/elf"
	"fmt"
	"io"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/di/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/go-delve/delve/pkg/dwarf/godwarf"
)
 
func getTypeMap(dwarfData *dwarf.Data, targetFunctions map[string]bool) (*ditypes.TypeMap, error) {
	return loadFunctionDefinitions(dwarfData, targetFunctions)
}

var dwarfMap = make(map[string]*dwarf.Data, 0)

func loadFunctionDefinitions(dwarfData *dwarf.Data, targetFunctions map[string]bool) (*ditypes.TypeMap, error) {

	entryReader := dwarfData.Reader()
	typeReader := dwarfData.Reader()
	readingAFunction := false
	var funcName string

	var result = ditypes.TypeMap{
		Functions:        make(map[string][]ditypes.Parameter),
		InlinedFunctions: make(map[uint64][]*dwarf.Entry),
	}

	var (
		name       string
		typeFields *ditypes.Parameter
	)

entryLoop:
	for {
		entry, err := entryReader.Next()
		if err == io.EOF || entry == nil {
			break
		}

		if entryIsEmpty(entry) {
			readingAFunction = false
			continue entryLoop
		}

		if entry.Tag == dwarf.TagCompileUnit {

			name, ok := entry.Val(dwarf.AttrName).(string)
			if !ok {
				continue entryLoop
			}
			ranges, err := dwarfData.Ranges(entry)
			if err != nil {
				log.Printf("couldnt retrieve ranges for compile unit %s: %s", name, err)
				continue entryLoop
			}

			for i := range ranges {
				result.DeclaredFiles = append(result.DeclaredFiles, &ditypes.LowPCEntry{
					LowPC: ranges[i][0],
					Entry: entry,
				})
			}
		}

		if entry.Tag == dwarf.TagInlinedSubroutine {
			// This is a inlined function
			for i := range entry.Field {
				// Find it's high program counter (where it exits in the parent routine)
				if entry.Field[i].Attr == dwarf.AttrHighpc {

					// The field for HighPC can be a constant or address, which are int64 and uint64 respectively
					if entry.Field[i].Class == dwarf.ClassConstant {
						result.InlinedFunctions[uint64(entry.Field[i].Val.(int64))] =
							append([]*dwarf.Entry{entry}, result.InlinedFunctions[uint64(entry.Field[i].Val.(int64))]...)
					} else if entry.Field[i].Class == dwarf.ClassAddress {
						result.InlinedFunctions[entry.Field[i].Val.(uint64)] =
							append([]*dwarf.Entry{entry}, result.InlinedFunctions[entry.Field[i].Val.(uint64)]...)
					}
				}
			}
			continue entryLoop
		}

		if entry.Tag == dwarf.TagSubprogram {

			for _, field := range entry.Field {
				if field.Attr == dwarf.AttrLowpc {
					lowpc := field.Val.(uint64)
					result.FunctionsByPC = append(result.FunctionsByPC, &ditypes.LowPCEntry{LowPC: lowpc, Entry: entry})
				}
			}

			for _, field := range entry.Field {
				if field.Attr == dwarf.AttrName {
					funcName = field.Val.(string)
					if !targetFunctions[funcName] {
						continue entryLoop
					}
					result.Functions[funcName] = make([]ditypes.Parameter, 0)
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
				name = entry.Field[i].Val.(string)
			}

			// Collect information about the type of this ditypes.Parameter
			if entry.Field[i].Attr == dwarf.AttrType {

				typeReader.Seek(entry.Field[i].Val.(dwarf.Offset))
				typeEntry, err := typeReader.Next()
				if err != nil {
					return nil, err
				}

				if typeEntry.Tag == dwarf.TagTypedef {
					typeEntry, err = resolveTypedefToRealType(typeEntry, typeReader)
					if err != nil {
						return nil, err
					}
				}

				typeFields, err = expandTypeData(typeEntry.Offset, dwarfData)
				if err != nil {
					return nil, fmt.Errorf("error while parsing debug information: %w", err)
				}
			}
		}

		typeFields.Name = name

		// We've collected information about this ditypes.Parameter, append it to the slice of ditypes.Parameters for this function
		result.Functions[funcName] = append(result.Functions[funcName], *typeFields)
	}

	// Sort program counter slice for lookup when resolving pcs->functions
	slices.SortFunc(result.FunctionsByPC, func(a, b *ditypes.LowPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})
	slices.SortFunc(result.DeclaredFiles, func(a, b *ditypes.LowPCEntry) int {
		return cmp.Compare(b.LowPC, a.LowPC)
	})

	return &result, nil
}

func loadDWARF(binaryPath string) (*dwarf.Data, error) {
	if dwarfData, ok := dwarfMap[binaryPath]; ok {
		return dwarfData, nil
	}
	elfFile, err := elf.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("couldn't open elf binary: %w", err)
	}

	dwarfData, err := elfFile.DWARF()
	if err != nil {
		return nil, fmt.Errorf("couldn't retrieve debug info from elf: %w", err)
	}
	dwarfMap[binaryPath] = dwarfData
	return dwarfData, nil
}

func expandTypeData(offset dwarf.Offset, dwarfData *dwarf.Data) (*ditypes.Parameter, error) {
	typeReader := dwarfData.Reader()

	typeReader.Seek(offset)
	typeEntry, err := typeReader.Next()
	if err != nil {
		return nil, fmt.Errorf("could not get type entry: %w", err)
	}

	if typeEntry.Tag == dwarf.TagTypedef {
		typeEntry, err = resolveTypedefToRealType(typeEntry, typeReader)
		if err != nil {
			return nil, err
		}
	}

	typeName, typeSize, typeKind := getTypeEntryBasicInfo(typeEntry)
	typeHeader := ditypes.Parameter{
		Type:      typeName,
		TotalSize: typeSize,
		Kind:      typeKind,
	}

	if typeEntry.Tag == dwarf.TagStructType && typeName != "string" {
		structFields, err := getStructFields(typeEntry.Offset, dwarfData)
		if err != nil {
			return nil, fmt.Errorf("could not collect fields of struct type of ditypes.Parameter: %w", err)
		}
		typeHeader.ParameterPieces = structFields
	} else if typeEntry.Tag == dwarf.TagArrayType {
		arrayElements, err := getIndividualArrayElements(typeEntry.Offset, dwarfData)
		if err != nil {
			return nil, fmt.Errorf("could not get length of array: %w", err)
		}
		typeHeader.ParameterPieces = arrayElements
	} else if typeEntry.Tag == dwarf.TagPointerType {
		pointerElements, err := getPointerLayers(typeEntry.Offset, dwarfData)
		if err != nil {
			return nil, fmt.Errorf("could not find pointer type: %w", err)
		}
		typeHeader.Location.NeedsDereference = true
		typeHeader.ParameterPieces = pointerElements
	}

	return &typeHeader, nil
}

func getIndividualArrayElements(offset dwarf.Offset, dwarfData *dwarf.Data) ([]ditypes.Parameter, error) {

	savedArrayEntryOffset := offset
	typeReader := dwarfData.Reader()

	// Go to the entry of the array type to get the underlying type information
	typeReader.Seek(offset)
	typeEntry, err := typeReader.Next()
	if err != nil {
		return nil, fmt.Errorf("could not get array type entry: %w", err)
	}
	arrayElementTypeEntry, err := resolveTypedefToRealType(typeEntry, typeReader)
	if err != nil {
		return nil, err
	}

	elementFields, err := expandTypeData(arrayElementTypeEntry.Offset, dwarfData)
	if err != nil {
		return nil, err
	}

	elementTypeName, elementTypeSize, elementTypeKind := getTypeEntryBasicInfo(arrayElementTypeEntry)

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

	arrayElements := []ditypes.Parameter{}
	for h := 0; h < int(arrayLength); h++ {
		newParam := ditypes.Parameter{}
		copyTree(&newParam.ParameterPieces, &elementFields.ParameterPieces)
		newParam.Name = fmt.Sprintf("[%d]%s[%d]", arrayLength, elementTypeName, h)
		newParam.Kind = elementTypeKind
		newParam.TotalSize = elementTypeSize
		arrayElements = append(arrayElements, newParam)
	}

	return arrayElements, nil
}

func getStructFields(offset dwarf.Offset, dwarfData *dwarf.Data) ([]ditypes.Parameter, error) {
	inOrderReader := dwarfData.Reader()
	typeReader := dwarfData.Reader()

	structFields := []ditypes.Parameter{}
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
			return []ditypes.Parameter{}, err
		}

		if entryIsEmpty(fieldEntry) || fieldEntry.Tag != dwarf.TagMember {
			break
		}

		newStructField := ditypes.Parameter{}

		for i := range fieldEntry.Field {

			// Struct Field Name
			if fieldEntry.Field[i].Attr == dwarf.AttrName {
				newStructField.Name = fieldEntry.Field[i].Val.(string)
			}

			// Struct Field Type
			if fieldEntry.Field[i].Attr == dwarf.AttrType {
				typeReader.Seek(fieldEntry.Field[i].Val.(dwarf.Offset))
				typeEntry, err := typeReader.Next()
				if err != nil {
					return []ditypes.Parameter{}, err
				}

				if typeEntry.Tag == dwarf.TagTypedef {
					typeEntry, err = resolveTypedefToRealType(typeEntry, typeReader)
					if err != nil {
						return []ditypes.Parameter{}, err
					}
				}

				newStructField.Type, newStructField.TotalSize, newStructField.Kind = getTypeEntryBasicInfo(typeEntry)
				if typeEntry.Tag != dwarf.TagBaseType {
					field, err := expandTypeData(typeEntry.Offset, dwarfData)
					if err != nil {
						return []ditypes.Parameter{}, err
					}
					field.Name = newStructField.Name
					structFields = append(structFields, *field)
				} else {
					structFields = append(structFields, newStructField)
				}
			}
		}
	}
	return structFields, nil
}

func getPointerLayers(offset dwarf.Offset, dwarfData *dwarf.Data) ([]ditypes.Parameter, error) {
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

			underlyingType, err = expandTypeData(typeEntry.Offset, dwarfData)
			if err != nil {
				return nil, err
			}
		}
	}

	return []ditypes.Parameter{*underlyingType}, nil
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
			typeName = typeEntry.Field[i].Val.(string)
		}
		if typeEntry.Field[i].Attr == dwarf.AttrByteSize {
			typeSize = typeEntry.Field[i].Val.(int64)
		}
		if typeEntry.Field[i].Attr == godwarf.AttrGoKind {
			typeKind = uint(typeEntry.Field[i].Val.(int64))

			if typeEntry.Tag == dwarf.TagPointerType && typeKind == 0 {
				typeKind = uint(reflect.Pointer) // Temporary fix for bug: https://github.com/golang/go/issues/64231
			}
		}
	}
	return
}

func resolveTypedefToRealType(outerType *dwarf.Entry, reader *dwarf.Reader) (*dwarf.Entry, error) {
	for i := range outerType.Field {
		if outerType.Field[i].Attr == dwarf.AttrType {
			reader.Seek(outerType.Field[i].Val.(dwarf.Offset))
			realTypeEntry, err := reader.Next()
			if err != nil {
				return nil, fmt.Errorf("error while retrieving underlying type: %w", err)
			}

			if realTypeEntry.Tag == dwarf.TagTypedef {
				realTypeEntry, err = resolveTypedefToRealType(realTypeEntry, reader)
				if err != nil {
					return nil, err
				}
			}

			return realTypeEntry, nil
		}
	}

	return outerType, nil
}

func correctStructSizes(params []ditypes.Parameter) {
	for i := range params {
		correctStructSize(&params[i])
	}
}

// correctStructSize sets the size of structs to the number of fields in the struct
func correctStructSize(param *ditypes.Parameter) {
	if len(param.ParameterPieces) == 0 {
		return
	}

	if param.Kind == uint(reflect.Struct) || param.Kind == uint(reflect.Array) {
		param.TotalSize = int64(len(param.ParameterPieces))
	}

	for i := range param.ParameterPieces {
		correctStructSize(&param.ParameterPieces[i])
	}
}

func copyTree(dst, src *[]ditypes.Parameter) {

	if dst == nil || src == nil || len(*src) == 0 {
		return
	}

	*dst = make([]ditypes.Parameter, len(*src))
	copy(*dst, *src)

	for i := range *src {
		copyTree(&((*dst)[i].ParameterPieces), &((*src)[i].ParameterPieces))
	}
}
