// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// genintern generates InternStrings() methods for workloadmeta entity structs.
//
// It reads struct field tags of the form `intern:"true"` and generates methods
// that deduplicate strings via the standard library's unique.Make, reducing
// heap memory on nodes with many pods/containers sharing labels, namespaces, etc.
//
// Supported field types with `intern:"true"`:
//
//	string            → unique.Make(s).Value()
//	map[string]string → intern all keys and values
//	[]string          → intern each element
//	StructType        → call StructType.InternStrings() (if that type also has tagged fields)
//	*StructType       → nil-check, then call InternStrings()
//	[]StructType      → iterate and call InternStrings() on each element
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

func main() {
	var (
		inputFile  string
		outputFile string
	)
	flag.StringVar(&inputFile, "input", "types.go", "input Go source file with struct definitions")
	flag.StringVar(&outputFile, "output", "intern_generated.go", "output file for generated code")
	flag.Parse()

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, inputFile, nil, parser.ParseComments)
	if err != nil {
		log.Fatalf("failed to parse %s: %v", inputFile, err)
	}

	structs := collectStructs(f)

	// Phase 1: identify which types have directly tagged fields.
	directlyInternable := make(map[string]bool)
	for name, fields := range structs {
		for _, field := range fields {
			if field.internTag != "" {
				directlyInternable[name] = true
				break
			}
		}
	}

	// Phase 2: propagate — a struct is internable if it has tagged fields OR
	// any of its fields reference an internable struct type.
	internable := make(map[string]bool)
	maps.Copy(internable, directlyInternable)
	changed := true
	for changed {
		changed = false
		for name, fields := range structs {
			if internable[name] {
				continue
			}
			for _, field := range fields {
				if internable[field.baseTypeName] {
					internable[name] = true
					changed = true
					break
				}
			}
		}
	}

	// Phase 3: generate code.
	var buf bytes.Buffer
	buf.WriteString(header(f.Name.Name))

	// Sort type names for deterministic output.
	var names []string
	for name := range internable {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		fields := structs[name]
		writeInternMethod(&buf, name, fields, internable)
	}

	// Format and write output.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Write unformatted for debugging.
		os.WriteFile(outputFile, buf.Bytes(), 0644)
		log.Fatalf("generated code failed gofmt: %v (raw output written to %s)", err, outputFile)
	}

	outPath := filepath.Join(filepath.Dir(inputFile), outputFile)
	if err := os.WriteFile(outPath, formatted, 0644); err != nil {
		log.Fatalf("failed to write %s: %v", outPath, err)
	}

	fmt.Printf("genintern: wrote %s (%d internable types)\n", outPath, len(internable))
}

// fieldInfo holds metadata about a single struct field relevant to interning.
type fieldInfo struct {
	name         string // field name (empty for embedded)
	typeName     string // full type as written in source (e.g., "map[string]string", "*Foo", "[]Bar")
	baseTypeName string // the struct name if this is a struct/ptr/slice-of-struct (e.g., "Foo", "Bar")
	isPointer    bool   // *T
	isSlice      bool   // []T
	isMap        bool   // map[K]V
	isString     bool
	isStringMap  bool // map[string]string
	isStringSlc  bool // []string
	isEmbedded   bool
	internTag    string // value of `intern:` struct tag ("true" or "")
}

// collectStructs extracts all named struct types and their fields from a file.
func collectStructs(f *ast.File) map[string][]fieldInfo {
	result := make(map[string][]fieldInfo)

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			var fields []fieldInfo
			for _, astField := range structType.Fields.List {
				fi := analyzeField(astField)
				fields = append(fields, fi...)
			}
			result[typeSpec.Name.Name] = fields
		}
	}

	return result
}

// analyzeField extracts fieldInfo from an ast.Field.
func analyzeField(f *ast.Field) []fieldInfo {
	tag := ""
	if f.Tag != nil {
		tag = parseInternTag(f.Tag.Value)
	}

	typeStr := typeToString(f.Type)
	base := baseStructName(f.Type)
	isPtr := isPointerType(f.Type)
	isSlc := isSliceType(f.Type)
	isMap := isMapType(f.Type)
	isStr := typeStr == "string"
	isStrMap := typeStr == "map[string]string"
	isStrSlc := typeStr == "[]string"

	// Embedded field (no names).
	if len(f.Names) == 0 {
		return []fieldInfo{{
			name:         base,
			typeName:     typeStr,
			baseTypeName: base,
			isPointer:    isPtr,
			isSlice:      isSlc,
			isMap:        isMap,
			isString:     isStr,
			isStringMap:  isStrMap,
			isStringSlc:  isStrSlc,
			isEmbedded:   true,
			internTag:    tag,
		}}
	}

	var result []fieldInfo
	for _, name := range f.Names {
		result = append(result, fieldInfo{
			name:         name.Name,
			typeName:     typeStr,
			baseTypeName: base,
			isPointer:    isPtr,
			isSlice:      isSlc,
			isMap:        isMap,
			isString:     isStr,
			isStringMap:  isStrMap,
			isStringSlc:  isStrSlc,
			internTag:    tag,
		})
	}
	return result
}

// parseInternTag extracts the value of the `intern` key from a raw struct tag string.
func parseInternTag(rawTag string) string {
	trimmed := strings.Trim(rawTag, "`")
	st := reflect.StructTag(trimmed)
	return st.Get("intern")
}

// writeInternMethod generates an InternStrings() method for a given struct type.
func writeInternMethod(buf *bytes.Buffer, typeName string, fields []fieldInfo, internable map[string]bool) {
	receiver := strings.ToLower(typeName[:1])

	fmt.Fprintf(buf, "// InternStrings deduplicates all string-typed fields tagged with `intern:\"true\"`.\n")
	fmt.Fprintf(buf, "// Nested struct fields whose types are also internable are recursed into automatically.\n")
	fmt.Fprintf(buf, "func (%s *%s) InternStrings() {\n", receiver, typeName)

	for _, fi := range fields {
		accessor := receiver + "." + fi.name

		// Directly tagged fields.
		if fi.internTag == "true" {
			switch {
			case fi.isString:
				fmt.Fprintf(buf, "\t%s = internString(%s)\n", accessor, accessor)
			case fi.isStringMap:
				fmt.Fprintf(buf, "\t%s = internStringMap(%s)\n", accessor, accessor)
			case fi.isStringSlc:
				fmt.Fprintf(buf, "\tinternStringSlice(%s)\n", accessor)
			}
		}

		// Recurse into fields whose type is an internable struct.
		if fi.baseTypeName == "" || !internable[fi.baseTypeName] {
			continue
		}

		switch {
		case fi.isEmbedded && fi.isPointer:
			fmt.Fprintf(buf, "\tif %s != nil {\n", accessor)
			fmt.Fprintf(buf, "\t\t%s.InternStrings()\n", accessor)
			fmt.Fprintf(buf, "\t}\n")
		case fi.isEmbedded:
			fmt.Fprintf(buf, "\t%s.InternStrings()\n", accessor)
		case fi.isPointer:
			fmt.Fprintf(buf, "\tif %s != nil {\n", accessor)
			fmt.Fprintf(buf, "\t\t%s.InternStrings()\n", accessor)
			fmt.Fprintf(buf, "\t}\n")
		case fi.isSlice && fi.baseTypeName != "":
			fmt.Fprintf(buf, "\tfor i := range %s {\n", accessor)
			fmt.Fprintf(buf, "\t\t%s[i].InternStrings()\n", accessor)
			fmt.Fprintf(buf, "\t}\n")
		case !fi.isSlice && !fi.isPointer && !fi.isMap:
			fmt.Fprintf(buf, "\t%s.InternStrings()\n", accessor)
		}
	}

	fmt.Fprintf(buf, "}\n\n")
}

func header(pkgName string) string {
	return fmt.Sprintf(`// Code generated by genintern; DO NOT EDIT.

package %s

import "unique"

func internString(s string) string {
	if s == "" {
		return ""
	}
	return unique.Make(s).Value()
}

func internStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[internString(k)] = internString(v)
	}
	return out
}

func internStringSlice(s []string) {
	for i, v := range s {
		s[i] = internString(v)
	}
}

`, pkgName)
}

// --- AST helpers ---

func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + typeToString(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", typeToString(t.Len), typeToString(t.Elt))
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeToString(t.Key), typeToString(t.Value))
	case *ast.SelectorExpr:
		return typeToString(t.X) + "." + t.Sel.Name
	case *ast.BasicLit:
		return t.Value
	case *ast.InterfaceType:
		return "interface{}"
	default:
		return ""
	}
}

// baseStructName returns the underlying struct name from a type expression,
// stripping pointer and slice wrappers. Returns "" for non-ident types.
func baseStructName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if len(t.Name) > 0 && t.Name[0] >= 'A' && t.Name[0] <= 'Z' {
			return t.Name
		}
		return ""
	case *ast.StarExpr:
		return baseStructName(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return baseStructName(t.Elt)
		}
		return ""
	default:
		return ""
	}
}

func isPointerType(expr ast.Expr) bool {
	_, ok := expr.(*ast.StarExpr)
	return ok
}

func isSliceType(expr ast.Expr) bool {
	arr, ok := expr.(*ast.ArrayType)
	return ok && arr.Len == nil
}

func isMapType(expr ast.Expr) bool {
	_, ok := expr.(*ast.MapType)
	return ok
}
