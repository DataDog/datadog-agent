// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/structtag"
	"golang.org/x/tools/go/packages"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors/common"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/generators/accessors/doc"
)

const (
	pkgPrefix = "github.com/DataDog/datadog-agent/pkg/security/secl"
)

var (
	filename          string
	modelPkgName      string
	output            string
	verbose           bool
	mock              bool
	genDoc            bool
	packagesLookupMap map[string]*types.Package
	buildTags         string
)

var module *common.Module

func resolveSymbol(pkg, symbol string) (types.Object, error) {
	if typePackage, found := packagesLookupMap[pkg]; found {
		return typePackage.Scope().Lookup(symbol), nil
	}

	return nil, fmt.Errorf("failed to retrieve package info for %s", pkg)
}

func origTypeToBasicType(pkgName string, kind string) string {
	if pkgName != "" {
		kind = fmt.Sprintf("%s.%s", pkgName, kind)
	}

	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "int"
	case "bool":
		return "bool"
	case "string", "eval.StringBuilder":
		return "string"
	}
	return module.SourcePkgPrefix + kind
}

func origType(pkgName string, kind string) string {
	if pkgName != "" {
		kind = fmt.Sprintf("%s.%s", pkgName, kind)
	}

	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "string", "bool", "eval.StringBuilder":
		return kind
	default:
		return module.SourcePkgPrefix + kind
	}
}

func handleBasic(name, alias, kind, event string, iterator *common.StructField, isArray bool, opOverrides string, commentText string, isPointer bool, exported bool) {
	fmt.Printf("handleBasic %s %s\n", name, kind)

	basicType := origTypeToBasicType("", kind)
	module.Fields[alias] = &common.StructField{
		Name:          name,
		BasicType:     basicType,
		ReturnType:    basicType,
		IsArray:       strings.HasPrefix(kind, "[]") || isArray,
		Event:         event,
		OrigType:      kind,
		IsOrigTypePtr: isPointer,
		Iterator:      iterator,
		CommentText:   commentText,
		OpOverrides:   opOverrides,
		Exported:      exported,
	}

	module.EventTypes[event] = true
}

func handleField(astFile *ast.File, name, alias, prefix, aliasPrefix, modelPkgName, pkgName, kind, event string, iterator *common.StructField, dejavu map[string]bool, isArray bool, opOverride string, commentText string, isPointer bool, exported bool) error {
	fmt.Printf("handleField fieldName `%s`, alias `%s`, prefix `%s`, aliasPrefix `%s`, pkgName `%s`, fieldType, `%s`\n", name, alias, prefix, aliasPrefix, pkgName, kind)

	fqKind := kind
	if pkgName != "" {
		fqKind = fmt.Sprintf("%s.%s", pkgName, kind)
	}

	switch fqKind {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "eval.StringBuilder":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}

		handleBasic(name, alias, fqKind, event, iterator, isArray, opOverride, commentText, isPointer, exported)

	default:
		symbol, err := resolveSymbol(modelPkgName, kind)
		if err != nil {
			return fmt.Errorf("failed to resolve symbol for %s in %s: %s", kind, modelPkgName, err)
		}
		if symbol == nil {
			return fmt.Errorf("failed to resolve symbol for %s in %s", kind, modelPkgName)
		}

		if prefix != "" {
			prefix = prefix + "." + name
			aliasPrefix = aliasPrefix + "." + alias
		} else {
			prefix = name
			aliasPrefix = alias
		}

		spec := astFile.Scope.Lookup(kind)
		handleSpec(astFile, spec.Decl, prefix, aliasPrefix, event, iterator, dejavu, exported)
	}

	return nil
}

func getFieldIdent(field *ast.Field) (string, *ast.Ident, bool, bool) {
	if fieldType, ok := field.Type.(*ast.Ident); ok {
		return "", fieldType, false, false
	} else if fieldType, ok := field.Type.(*ast.StarExpr); ok {
		if ident, ok := fieldType.X.(*ast.Ident); ok {
			return "", ident, true, false
		}
		if ident, ok := fieldType.X.(*ast.SelectorExpr); ok {
			pkgName := fmt.Sprintf("%v", ident.X)

			return pkgName, ident.Sel, true, false
		}
	} else if ft, ok := field.Type.(*ast.ArrayType); ok {
		if ident, ok := ft.Elt.(*ast.Ident); ok {
			return "", ident, false, true
		}
	}
	return "", nil, false, false
}

type seclField struct {
	name     string
	iterator string
	handler  string
}

func parseHandler(handler string) (string, int64) {
	els := strings.Split(handler, ":")
	handler = els[0]

	var weight int64
	var err error
	if len(els) > 1 {
		weight, err = strconv.ParseInt(els[1], 10, 64)
		if err != nil {
			log.Panicf("unable to parse weight: %s", els[1])
		}
	}

	return handler, weight
}

func handleSpec(astFile *ast.File, spec interface{}, prefix, aliasPrefix, event string, iterator *common.StructField, dejavu map[string]bool, exported bool) {
	fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s, event %s, iterator %+v\n", spec, prefix, aliasPrefix, event, iterator)

	if typeSpec, ok := spec.(*ast.TypeSpec); ok {
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
			//FIELD:
			for _, field := range structType.Fields.List {
				fieldCommentText := field.Comment.Text()

				fieldIterator := iterator

				var tag reflect.StructTag
				if field.Tag != nil {
					tag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
				}

				if e, ok := tag.Lookup("event"); ok {
					event = e
					module.EventTypeDocs[e] = fieldCommentText
				}

				if isEmbedded := len(field.Names) == 0; !isEmbedded {
					seclFieldName := field.Names[0].Name

					if !unicode.IsUpper(rune(seclFieldName[0])) {
						continue
					}

					if dejavu[seclFieldName] {
						continue
					}

					var opOverrides string
					var fields []seclField
					pkgName, fieldType, isPointer, isArray := getFieldIdent(field)

					var weight int64
					if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
						for _, tag := range tags.Tags() {
							switch tag.Key {
							case "field":
								splitted := strings.SplitN(tag.Value(), ",", 3)
								alias := splitted[0]

								field := seclField{name: alias}
								if len(splitted) > 1 {
									field.handler, weight = parseHandler(splitted[1])
								}
								if len(splitted) > 2 {
									field.iterator, weight = parseHandler(splitted[2])
								}

								fields = append(fields, field)
							case "op_override":
								opOverrides = tag.Value()
							}
						}
					} else {
						fields = append(fields, seclField{name: seclFieldName})
					}

					for _, seclField := range fields {
						fieldAlias := seclField.name
						alias := fieldAlias
						if aliasPrefix != "" {
							alias = aliasPrefix + "." + fieldAlias
						}

						if iterator := seclField.iterator; iterator != "" {
							module.Iterators[alias] = &common.StructField{
								Name:          fmt.Sprintf("%s.%s", prefix, seclFieldName),
								ReturnType:    origType(pkgName, iterator),
								Event:         event,
								OrigType:      origType(pkgName, fieldType.Name),
								IsOrigTypePtr: isPointer,
								IsArray:       isArray,
								Weight:        weight,
								CommentText:   fieldCommentText,
								OpOverrides:   opOverrides,
								Exported:      exported && fieldAlias != "-",
							}

							fieldIterator = module.Iterators[alias]
						}

						if handler := seclField.handler; handler != "" {
							if aliasPrefix != "" {
								fieldAlias = aliasPrefix + "." + fieldAlias
							}

							module.Fields[fieldAlias] = &common.StructField{
								Prefix:        prefix,
								Name:          fmt.Sprintf("%s.%s", prefix, seclFieldName),
								BasicType:     origTypeToBasicType(pkgName, fieldType.Name),
								Struct:        typeSpec.Name.Name,
								Handler:       handler,
								ReturnType:    origTypeToBasicType(pkgName, fieldType.Name),
								Event:         event,
								OrigType:      origType(pkgName, fieldType.Name),
								IsOrigTypePtr: isPointer,
								Iterator:      fieldIterator,
								IsArray:       isArray,
								Weight:        weight,
								CommentText:   fieldCommentText,
								OpOverrides:   opOverrides,
								Exported:      exported && fieldAlias != "-",
							}

							module.EventTypes[event] = true
							delete(dejavu, seclFieldName)

							continue
						}

						dejavu[seclFieldName] = true

						if fieldType != nil {
							if err := handleField(astFile, seclFieldName, fieldAlias, prefix, aliasPrefix, modelPkgName, pkgName, fieldType.Name, event, fieldIterator, dejavu, false, opOverrides, fieldCommentText, isPointer, exported && fieldAlias != "-"); err != nil {
								log.Print(err)
							}

							delete(dejavu, seclFieldName)
						}

						if verbose {
							log.Printf("Don't know what to do with %s: %s", seclFieldName, spew.Sdump(field.Type))
						}
					}
				} else {
					if fieldTag, found := tag.Lookup("field"); found && fieldTag == "-" {
						exported = false
					}

					// Embedded field
					ident, _ := field.Type.(*ast.Ident)
					if starExpr, ok := field.Type.(*ast.StarExpr); ident == nil && ok {
						ident, _ = starExpr.X.(*ast.Ident)
					}

					if ident != nil {
						embedded := astFile.Scope.Lookup(ident.Name)
						if embedded != nil {
							log.Printf("Embedded struct %s", ident.Name)
							handleSpec(astFile, embedded.Decl, prefix+"."+ident.Name, aliasPrefix, event, fieldIterator, dejavu, exported)
						}
					}
				}
			}
		} else {
			log.Printf("Don't know what to do with %s (%s)", typeSpec.Name, spew.Sdump(typeSpec))
		}
	}
}

func parseFile(filename string, pkgName string) (*common.Module, error) {
	cfg := packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		BuildFlags: []string{"-mod=mod", fmt.Sprintf("-tags=%s", buildTags)},
	}

	pkgs, err := packages.Load(&cfg, filename)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 || len(pkgs[0].Syntax) == 0 {
		return nil, errors.New("failed to get syntax from parse file")
	}

	pkg := pkgs[0]
	astFile := pkg.Syntax[0]

	packagesLookupMap = make(map[string]*types.Package)
	for _, typePackage := range pkg.Imports {
		p := typePackage.Types
		packagesLookupMap[p.Path()] = p
	}
	packagesLookupMap[pkgName] = pkg.Types

	splittedBuildTags := strings.Split(buildTags, ",")
	var buildTags []string
	for _, tag := range splittedBuildTags {
		if tag != "" {
			buildTags = append(buildTags, fmt.Sprintf("+build %s", tag))
		}
	}
	for _, comment := range astFile.Comments {
		if strings.HasPrefix(comment.Text(), "+build ") {
			buildTags = append(buildTags, comment.Text())
		}
	}

	moduleName := path.Base(path.Dir(output))
	if moduleName == "." {
		moduleName = path.Base(pkgName)
	}

	module = &common.Module{
		Name:          moduleName,
		SourcePkg:     pkgName,
		TargetPkg:     pkgName,
		BuildTags:     buildTags,
		Fields:        make(map[string]*common.StructField),
		Iterators:     make(map[string]*common.StructField),
		EventTypes:    make(map[string]bool),
		EventTypeDocs: make(map[string]string),
		Mock:          mock,
	}

	// If the target package is different from the model package
	if module.Name != path.Base(pkgName) {
		module.SourcePkgPrefix = path.Base(pkgName) + "."
		module.TargetPkg = path.Clean(path.Join(pkgName, path.Dir(output)))
	}

	for _, decl := range astFile.Decls {
		if decl, ok := decl.(*ast.GenDecl); ok {
			genaccessors := false
			if decl.Doc != nil {
				for _, doc := range decl.Doc.List {
					if genaccessors = strings.Index(doc.Text, "genaccessors") != -1; genaccessors {
						break
					}
				}
			}
			if !genaccessors {
				continue
			}

			for _, spec := range decl.Specs {
				if typeSpec, ok := spec.(*ast.TypeSpec); ok {
					handleSpec(astFile, typeSpec, "", "", "", nil, make(map[string]bool), true)
				}
			}
		}
	}

	return module, nil
}

var FuncMap = map[string]interface{}{
	"TrimPrefix": strings.TrimPrefix,
}

func main() {
	var err error
	tmpl := template.Must(template.New("header").Funcs(FuncMap).Parse(`{{- range .BuildTags }}// {{.}}{{end}}

// Code generated - DO NOT EDIT.

package {{.Name}}

import (
	"reflect"
	"unsafe"

	{{if ne $.SourcePkg $.TargetPkg}}"{{.SourcePkg}}"{{end}}
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// suppress unused package warning
var (
	_ *unsafe.Pointer
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {
	{{- range $Name, $Field := .Iterators}}
	{{- if $Field.Exported}}
	case "{{$Name}}":
		return &{{$Field.ReturnType}}{}, nil
	{{- end}}
	{{- end}}
	}

	return nil, &eval.ErrIteratorNotSupported{Field: field}
}

func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{
		{{- range $Name, $Exists := .EventTypes}}
			{{- if and (ne $Name "*") (ne $Name "")}}
			eval.EventType("{{$Name}}"),
			{{- end -}}
		{{- end}}
	}
}

func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	{{$Mock := .Mock}}
	{{range $Name, $Field := .Fields}}
	{{- if $Field.Exported}}
	{{$EvaluatorType := "eval.StringEvaluator"}}
	{{if or $Field.Iterator $Field.IsArray}}
		{{$EvaluatorType = "eval.StringArrayEvaluator"}}
	{{end}}
	{{if eq $Field.ReturnType "int"}}
		{{$EvaluatorType = "eval.IntEvaluator"}}
		{{if or $Field.Iterator $Field.IsArray}}
			{{$EvaluatorType = "eval.IntArrayEvaluator"}}
		{{end}}
	{{else if eq $Field.ReturnType "bool"}}
		{{$EvaluatorType = "eval.BoolEvaluator"}}
		{{if or $Field.Iterator $Field.IsArray}}
			{{$EvaluatorType = "eval.BoolArrayEvaluator"}}
		{{end}}
	{{end}}

	case "{{$Name}}":
		return &{{$EvaluatorType}}{
			{{- if and $Field.OpOverrides (not $Mock)}}
			OpOverrides: {{$Field.OpOverrides}},
			{{- end}}
			{{- if $Field.Iterator}}
				{{- $ArrayPrefix := ""}}
				{{- if $Field.IsArray}}
					{{$ArrayPrefix = "[]"}}
				{{end}}
				EvalFnc: func(ctx *eval.Context) []{{$Field.ReturnType}} {
					{{- if not $Mock }}
					if ptr := ctx.Cache[field]; ptr != nil {
						if result := (*[]{{$Field.ReturnType}})(ptr); result != nil {
							return *result
						}
					}
					{{end -}}

					var results []{{$Field.ReturnType}}

					iterator := &{{$Field.Iterator.ReturnType}}{}

					value := iterator.Front(ctx)
					for value != nil {
						var result {{$ArrayPrefix}}{{$Field.ReturnType}}

						{{if $Field.Iterator.IsOrigTypePtr}}
							element := (*{{$Field.Iterator.OrigType}})(value)
						{{else}}
							elementPtr := (*{{$Field.Iterator.OrigType}})(value)
							element := *elementPtr
						{{end}}

						{{$SubName := $Field.Iterator.Name | TrimPrefix $Field.Name}}

						{{$Return := $SubName | printf "element%s"}}
						{{if and (ne $Field.Handler "") (not $Mock) }}
							{{$Handler := $Field.Iterator.Name | TrimPrefix $Field.Handler}}
							{{$Return = print "(*Event)(ctx.Object)." $Handler "(&element." $Field.Struct ")"}}
						{{end}}

						{{if eq $Field.ReturnType "int"}}
							result = int({{$Return}})
						{{else}}
							{{if eq $Field.OrigType "eval.StringBuilder"}}
							result = {{$Return}}.String()
							{{else}}
							result = {{$Return}}
							{{end}}
						{{end}}

						{{if eq $ArrayPrefix ""}}
						results = append(results, result)
						{{else}}
						results = append(results, result...)
						{{end}}

						value = iterator.Next()
					}

					{{- if not $Mock }}
					ctx.Cache[field] = unsafe.Pointer(&results)
					{{end}}

					return results
				},
			{{- else}}
				{{- $ArrayPrefix := ""}}
				{{- $ReturnType := $Field.ReturnType}}
				{{- if $Field.IsArray}}
					{{$ArrayPrefix = "[]"}}
				{{end}}
				EvalFnc: func(ctx *eval.Context) {{$ArrayPrefix}}{{$ReturnType}} {
					{{$Return := $Field.Name | printf "(*Event)(ctx.Object).%s"}}
					{{- if and (ne $Field.Handler "") (not $Mock)}}
						{{$Return = print "(*Event)(ctx.Object)." $Field.Handler "(&(*Event)(ctx.Object)." $Field.Prefix ")"}}
					{{end}}

					{{- if eq $ReturnType "int"}}
						{{- if and ($Field.IsArray) (ne $Field.OrigType "int") }}
							result := make([]int, len({{$Return}}))
							for i, v := range {{$Return}} {
								result[i] = int(v)
							}
							return result
						{{- else}}
							{{- if ne $Field.OrigType "int"}}
								return int({{$Return}})
							{{- else}}
								return {{$Return}}
							{{end -}}
						{{end -}}
					{{- else}}
						{{if eq $Field.OrigType "eval.StringBuilder"}}
						return {{$Return}}.String()
						{{else}}
						return {{$Return}}
						{{end}}
					{{end -}}
				},
			{{end -}}
			Field: field,
			{{- if $Field.Iterator}}
				{{- if gt $Field.Weight 0}}
				Weight: {{$Field.Weight}} * eval.IteratorWeight,
				{{else}}
				Weight: eval.IteratorWeight,
				{{end}}
			{{else if $Field.Handler}}
				{{- if gt $Field.Weight 0}}
					Weight: {{$Field.Weight}} * eval.HandlerWeight,
				{{else}}
					Weight: eval.HandlerWeight,
				{{end -}}
			{{else}}
				Weight: eval.FunctionWeight,
			{{end}}
		}, nil
	{{end}}
	{{end}}	
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFields() []eval.Field {
	return []eval.Field{
		{{range $Name, $Field := .Fields}}
		{{- if $Field.Exported}}
			"{{$Name}}",
		{{- end}}		
		{{end}}
	}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
		{{$Mock := .Mock}}
		{{range $Name, $Field := .Fields}}
		{{- if $Field.Exported}}
		case "{{$Name}}":
		{{if $Field.Iterator}}
			{{- $ArrayPrefix := ""}}
			{{- if $Field.IsArray}}
				{{$ArrayPrefix = "[]"}}
			{{end}}

			var values []{{$Field.ReturnType}}

			ctx := eval.NewContext(unsafe.Pointer(e))

			iterator := &{{$Field.Iterator.ReturnType}}{}
			ptr := iterator.Front(ctx)

			for ptr != nil {
				{{if $Field.Iterator.IsOrigTypePtr}}
					element := (*{{$Field.Iterator.OrigType}})(ptr)
				{{else}}
					elementPtr := (*{{$Field.Iterator.OrigType}})(ptr)
					element := *elementPtr
				{{end}}

				{{$SubName := $Field.Iterator.Name | TrimPrefix $Field.Name}}

				{{$Return := $SubName | printf "element%s"}}
				{{if and (ne $Field.Handler "") (not $Mock) }}
					{{$Handler := $Field.Iterator.Name | TrimPrefix $Field.Handler}}
					{{$Return = print "(*Event)(ctx.Object)." $Handler "(&element." $Field.Struct ")"}}
				{{end}}

				{{if and (eq $Field.ReturnType "int") (ne $Field.OrigType "int")}}
					result := int({{$Return}})
				{{else}}
					{{if eq $Field.OrigType "eval.StringBuilder"}}
					result := {{$Return}}.String()
					{{else}}
					result := {{$Return}}
					{{end}}
				{{end}}

				{{if eq $ArrayPrefix ""}}
				values = append(values, result)
				{{else}}
				values = append(values, result...)
				{{end}}

				ptr = iterator.Next()
			}

			return values, nil
		{{else}}
			{{$Return := $Field.Name | printf "e.%s"}}
			{{if and (ne $Field.Handler "") (not $Mock)}}
				{{$Return = print "e." $Field.Handler "(&e." $Field.Prefix ")"}}
			{{end}}

			{{- $ArrayPrefix := ""}}
			{{- if $Field.IsArray}}
				{{$ArrayPrefix = "[]"}}
			{{end}}

			{{if eq $Field.ReturnType "string"}}
				{{if eq $Field.OrigType "eval.StringBuilder"}}
				return {{$Return}}.String(), nil
				{{else}}
				return {{$Return}}, nil
				{{end}}
			{{else if eq $Field.ReturnType "int"}}
				{{- if and ($Field.IsArray) (ne $Field.OrigType "int") }}
					result := make([]int, len({{$Return}}))
					for i, v := range {{$Return}} {
						result[i] = int(v)
					}
					return result, nil
				{{- else}}
					{{- if ne $Field.OrigType "int"}}
						return int({{$Return}}), nil
					{{- else}}
						return {{$Return}}, nil
					{{end -}}
				{{end -}}
			{{else if eq $Field.ReturnType "bool"}}
				return {{$Return}}, nil
			{{end}}

		{{end}}
		{{end}}
		{{end}}
		}

		return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {
	{{range $Name, $Field := .Fields}}
	{{- if $Field.Exported}}
	case "{{$Name}}":
		return "{{$Field.Event}}", nil
	{{end}}
	{{end}}
	}

	return "", &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
		{{range $Name, $Field := .Fields}}
		{{- if $Field.Exported}}

		case "{{$Name}}":
		{{if eq $Field.ReturnType "string"}}
			return reflect.String, nil
		{{else if eq $Field.ReturnType "int"}}
			return reflect.Int, nil
		{{else if eq $Field.ReturnType "bool"}}
			return reflect.Bool, nil
		{{end}}
		{{end}}
		{{end}}
		}

		return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
		{{range $Name, $Field := .Fields}}
		{{- if $Field.Exported}}		
		{{$FieldName := $Field.Name | printf "e.%s"}}
		case "{{$Name}}":
		{{if $Field.Iterator}}
			{{if $Field.Iterator.IsOrigTypePtr}}
				if e.{{$Field.Iterator.Name}} == nil {
					e.{{$Field.Iterator.Name}} = &{{$Field.Iterator.OrigType}}{}
				}
			{{end}}
		{{end}}
			var ok bool
		{{- if eq $Field.OrigType "string"}}
			str, ok := value.(string)
			if !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}
			{{- if $Field.IsArray}}
				{{$FieldName}} = append({{$FieldName}}, str)
			{{else}}
				{{$FieldName}} = str
			{{end}}
			return nil
		{{- else if eq $Field.OrigType "eval.StringBuilder"}}
			str, ok := value.(string)
			if !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}

			{{- if $Field.IsOrigTypePtr}}
			if {{$FieldName}} == nil {
				{{$FieldName}} = &eval.StringBuilder{}
			}
			{{end}}

			{{$FieldName}}.WriteString(str)
		{{- else if eq $Field.BasicType "int"}}
			v, ok := value.(int)
			if !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}
			{{- if $Field.IsArray}}
				{{$FieldName}} = append({{$FieldName}}, {{$Field.OrigType}}(v))
			{{else}}
				{{$FieldName}} = {{$Field.OrigType}}(v)
			{{end}}
			return nil
		{{else if eq $Field.BasicType "bool"}}
			if {{$FieldName}}, ok = value.(bool); !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}
			return nil
		{{end}}	
		{{end}}
		{{end}}
		}

		return &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) Init()  {
	{{range $Name, $Field := .Fields}}
		{{if not $Field.Iterator}}
			{{$FieldName := $Field.Name | printf "e.%s"}}
			{{- if eq $Field.OrigType "eval.StringBuilder" }}
				{{- if $Field.IsOrigTypePtr }}
				{{$FieldName}} = &eval.StringBuilder{}
				{{end -}}
			{{end}}
		{{end}}
	{{end}}
}

func (e *Event) Reset()  {
	{{range $Name, $Field := .Fields}}
		{{if not $Field.Iterator}}
			{{$FieldName := $Field.Name | printf "e.%s"}}
			{{- if eq $Field.OrigType "eval.StringBuilder" }}
				{{$FieldName}}.Reset()
			{{end}}
		{{end}}
	{{end}}
}

func (e *Event) ResetEventType(eventType eval.EventType)  {
	{{range $FieldName, $Field := $.Fields}}
	{{- if or (eq $Field.Event "*") (not $Field.Exported)}}
		{{- if not $Field.Iterator}}
			{{$FieldName := $Field.Name | printf "e.%s"}}
			{{- if eq $Field.OrigType "eval.StringBuilder" }}
				{{$FieldName}}.Reset()
			{{end}}
		{{end}}
	{{end}}
	{{end}}	

	switch eventType {
	{{- range $Name, $Exists := .EventTypes}}
	{{- if ne $Name "*"}}
		case "{{$Name}}":
			{{range $FieldName, $Field := $.Fields}}
			{{- if eq $Field.Event $Name}}
				{{- if not $Field.Iterator}}
					{{$FieldName := $Field.Name | printf "e.%s"}}
					{{- if eq $Field.OrigType "eval.StringBuilder" }}
						{{$FieldName}}.Reset()
					{{end}}
				{{end}}
			{{end}}
			{{end}}
	{{- end}}
	{{- end}}
	}
}
`))

	os.Remove(output)

	module, err = parseFile(filename, modelPkgName)
	if err != nil {
		panic(err)
	}

	if genDoc {
		if err := doc.GenerateDocJSON(module, output); err != nil {
			panic(err)
		}
		return
	}

	tmpfile, err := os.CreateTemp(path.Dir(output), "accessors")
	if err != nil {
		log.Fatal(err)
	}

	if err := tmpl.Execute(tmpfile, module); err != nil {
		panic(err)
	}

	if err := tmpfile.Close(); err != nil {
		panic(err)
	}

	cmd := exec.Command("gofmt", "-s", "-w", tmpfile.Name())
	if err := cmd.Run(); err != nil {
		panic(err)
	}

	if err := os.Rename(tmpfile.Name(), output); err != nil {
		panic(err)
	}
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Be verbose")
	flag.BoolVar(&mock, "mock", false, "Mock accessors")
	flag.BoolVar(&genDoc, "doc", false, "Generate documentation JSON")
	flag.StringVar(&filename, "input", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&modelPkgName, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
