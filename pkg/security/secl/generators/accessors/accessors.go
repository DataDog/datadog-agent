// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors/common"
	"github.com/DataDog/datadog-agent/pkg/security/secl/generators/accessors/doc"
	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/structtag"
	"golang.org/x/tools/go/loader"

	"github.com/alecthomas/jsonschema"
)

const (
	pkgPrefix = "github.com/DataDog/datadog-agent/pkg/security"
)

var (
	filename  string
	pkgname   string
	output    string
	strict    bool
	verbose   bool
	mock      bool
	genDoc    bool
	program   *loader.Program
	packages  map[string]*types.Package
	buildTags string
)

var module *common.Module

func resolveSymbol(pkg, symbol string) (types.Object, error) {
	if typePackage, found := packages[pkg]; found {
		return typePackage.Scope().Lookup(symbol), nil
	}

	return nil, fmt.Errorf("Failed to retrieve package info for %s", pkg)
}

func origTypeToBasicType(kind string) string {
	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "int"
	}
	return kind
}

func handleBasic(name, alias, kind, event string, iterator *common.StructField, isArray bool, commentText string) {
	fmt.Printf("handleBasic %s %s\n", name, kind)

	basicType := origTypeToBasicType(kind)
	module.Fields[alias] = &common.StructField{
		Name:        name,
		BasicType:   basicType,
		ReturnType:  basicType,
		IsArray:     strings.HasPrefix(kind, "[]") || isArray,
		Event:       event,
		OrigType:    kind,
		Iterator:    iterator,
		CommentText: commentText,
	}

	module.EventTypes[event] = true
}

func handleField(astFile *ast.File, name, alias, prefix, aliasPrefix, pkgName string, fieldType *ast.Ident, event string, iterator *common.StructField, dejavu map[string]bool, isArray bool, commentText string) error {
	fmt.Printf("handleField fieldName %s, alias %s, prefix %s, aliasPrefix %s, pkgName %s, fieldType, %s\n", name, alias, prefix, aliasPrefix, pkgName, fieldType)

	switch fieldType.Name {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}
		handleBasic(name, alias, fieldType.Name, event, iterator, isArray, commentText)

	default:
		symbol, err := resolveSymbol(pkgName, fieldType.Name)
		if err != nil {
			return fmt.Errorf("failed to resolve symbol for %+v in %s: %s", fieldType, pkgName, err)
		}
		if symbol == nil {
			return fmt.Errorf("failed to resolve symbol for %+v in %s", fieldType, pkgName)
		}

		if prefix != "" {
			prefix = prefix + "." + name
			aliasPrefix = aliasPrefix + "." + alias
		} else {
			prefix = name
			aliasPrefix = alias
		}

		spec := astFile.Scope.Lookup(fieldType.Name)
		handleSpec(astFile, spec.Decl, prefix, aliasPrefix, event, iterator, dejavu)
	}

	return nil
}

func getFieldIdent(field *ast.Field) (ident *ast.Ident, isPointer, isArray bool) {
	if fieldType, ok := field.Type.(*ast.Ident); ok {
		return fieldType, false, false
	} else if fieldType, ok := field.Type.(*ast.StarExpr); ok {
		if ident, ok := fieldType.X.(*ast.Ident); ok {
			return ident, true, false
		}
	} else if ft, ok := field.Type.(*ast.ArrayType); ok {
		if ident, ok := ft.Elt.(*ast.Ident); ok {
			return ident, false, true
		}
	}
	return nil, false, false
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

func handleSpec(astFile *ast.File, spec interface{}, prefix, aliasPrefix, event string, iterator *common.StructField, dejavu map[string]bool) {
	fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s, event %s, iterator %+v\n", spec, prefix, aliasPrefix, event, iterator)

	if typeSpec, ok := spec.(*ast.TypeSpec); ok {
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
		FIELD:
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
					fieldName := field.Names[0].Name

					if !unicode.IsUpper(rune(fieldName[0])) {
						continue
					}

					if dejavu[fieldName] {
						continue
					}

					var fields []seclField
					fieldType, isPointer, isArray := getFieldIdent(field)

					var weight int64
					if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
						for _, fieldTag := range tags.Tags() {
							if fieldTag.Key == "field" {
								splitted := strings.SplitN(fieldTag.Value(), ",", 3)
								alias := splitted[0]
								if alias == "-" {
									continue FIELD
								}
								field := seclField{name: alias}
								if len(splitted) > 1 {
									field.handler, weight = parseHandler(splitted[1])
								}
								if len(splitted) > 2 {
									field.iterator, weight = parseHandler(splitted[2])
								}

								fields = append(fields, field)
							}
						}
					} else {
						fields = append(fields, seclField{name: fieldName})
					}

					for _, seclField := range fields {
						fieldAlias := seclField.name
						alias := fieldAlias
						if aliasPrefix != "" {
							alias = aliasPrefix + "." + fieldAlias
						}

						if iterator := seclField.iterator; iterator != "" {
							qualifiedType := func(t string) string {
								switch t {
								case "int", "string", "bool":
									return t
								default:
									return module.SourcePkgPrefix + t
								}
							}

							module.Iterators[alias] = &common.StructField{
								Name:          fmt.Sprintf("%s.%s", prefix, fieldName),
								ReturnType:    qualifiedType(iterator),
								Event:         event,
								OrigType:      qualifiedType(fieldType.Name),
								IsOrigTypePtr: isPointer,
								IsArray:       isArray,
								Weight:        weight,
								CommentText:   fieldCommentText,
							}

							fieldIterator = module.Iterators[alias]
						}

						if handler := seclField.handler; handler != "" {
							if aliasPrefix != "" {
								fieldAlias = aliasPrefix + "." + fieldAlias
							}

							module.Fields[fieldAlias] = &common.StructField{
								Prefix:      prefix,
								Name:        fmt.Sprintf("%s.%s", prefix, fieldName),
								BasicType:   origTypeToBasicType(fieldType.Name),
								Struct:      typeSpec.Name.Name,
								Handler:     handler,
								ReturnType:  origTypeToBasicType(fieldType.Name),
								Event:       event,
								OrigType:    fieldType.Name,
								Iterator:    fieldIterator,
								IsArray:     isArray,
								Weight:      weight,
								CommentText: fieldCommentText,
							}

							module.EventTypes[event] = true
							delete(dejavu, fieldName)

							continue
						}

						dejavu[fieldName] = true

						if fieldType != nil {
							if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, pkgname, fieldType, event, fieldIterator, dejavu, false, fieldCommentText); err != nil {
								log.Print(err)
							}

							delete(dejavu, fieldName)
						}

						if strict {
							log.Panicf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
						}
						if verbose {
							log.Printf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
						}
					}
				} else {
					if fieldTag, found := tag.Lookup("field"); found && fieldTag == "-" {
						continue FIELD
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
							handleSpec(astFile, embedded.Decl, prefix+"."+ident.Name, aliasPrefix, event, fieldIterator, dejavu)
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
	buildContext := build.Default
	buildContext.BuildTags = append(buildContext.BuildTags, strings.Split(buildTags, ",")...)

	conf := loader.Config{
		ParserMode:  parser.ParseComments,
		AllowErrors: true,
		TypeChecker: types.Config{
			Error: func(err error) {
				if verbose {
					log.Print(err)
				}
			},
		},
		Build: &buildContext,
	}

	astFile, err := conf.ParseFile(filename, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to parse %s: %s", filename, err)
	}

	conf.Import(pkgName)

	program, err = conf.Load()
	if err != nil {
		return nil, fmt.Errorf("Failed to load %s (%s): %s", filename, pkgName, err)
	}

	packages = make(map[string]*types.Package, len(program.AllPackages))
	for typePackage := range program.AllPackages {
		packages[typePackage.Path()] = typePackage
	}

	var buildTags []string
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
					handleSpec(astFile, typeSpec, "", "", "", nil, make(map[string]bool))
				}
			}
		}
	}

	return module, nil
}

func genDocMain(module *common.Module, output string) error {
	seclJSONPath := path.Join(output, "secl.json")
	backendJSONPath := path.Join(output, "backend.schema.json")

	err := doc.GenerateDocJSON(module, seclJSONPath)
	if err != nil {
		return err
	}

	reflector := jsonschema.Reflector{
		ExpandedStruct: true,
		DoNotReference: false,
	}
	schema := reflector.Reflect(&probe.EventSerializer{})
	schemaJson, err := schema.MarshalJSON()
	if err != nil {
		return err
	}

	var out bytes.Buffer
	if err := json.Indent(&out, schemaJson, "", "  "); err != nil {
		return err
	}

	return ioutil.WriteFile(backendJSONPath, out.Bytes(), 0664)
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
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// suppress unused package warning
var (
	_ *unsafe.Pointer
)

func (m *Model) GetIterator(field eval.Field) (eval.Iterator, error) {
	switch field {
	{{range $Name, $Field := .Iterators}}
	case "{{$Name}}":
		return &{{$Field.ReturnType}}{}, nil
	{{end}}
	}

	return nil, &eval.ErrIteratorNotSupported{Field: field}
}

func (m *Model) GetEventTypes() []eval.EventType {
	return []eval.EventType{
		{{range $Name, $Exists := .EventTypes}}
			{{- if ne $Name "*"}}
			eval.EventType("{{$Name}}"),
			{{end -}}
		{{end}}
	}
}

func (m *Model) GetEvaluator(field eval.Field, regID eval.RegisterID) (eval.Evaluator, error) {
	switch field {
	{{$Mock := .Mock}}
	{{range $Name, $Field := .Fields}}
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
			{{- if $Field.Iterator}}
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
						var result {{$Field.ReturnType}}

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
							result = {{$Return}}
						{{end}}

						results = append(results, result)

						value = iterator.Next()
					}

					{{- if not $Mock }}
					ctx.Cache[field] = unsafe.Pointer(&results)
					{{end}}

					return results
				},
			{{- else}}
				{{- $ArrayPrefix := ""}}
				{{- if $Field.IsArray}}
					{{$ArrayPrefix = "[]"}}
				{{end}}
				EvalFnc: func(ctx *eval.Context) {{$ArrayPrefix}}{{$Field.ReturnType}} {
					{{$Return := $Field.Name | printf "(*Event)(ctx.Object).%s"}}
					{{- if and (ne $Field.Handler "") (not $Mock)}}
						{{$Return = print "(*Event)(ctx.Object)." $Field.Handler "(&(*Event)(ctx.Object)." $Field.Prefix ")"}}
					{{end}}

					{{- if eq $Field.ReturnType "int"}}
						{{- if and ($Field.IsArray) (ne $Field.OrigType "int") }}
							result := make([]int, len({{$Return}}))
							for i, v := range {{$Return}} {
								result[i] = in(v)
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
						return {{$Return}}
					{{end -}}
				},
			{{end -}}
			Field: field,
			{{- if $Field.Iterator}}
				Weight: eval.IteratorWeight,
			{{else if $Field.Handler}}
				{{- if gt $Field.Weight 0}}
					Weight: {{$Field.Weight}},
				{{else}}
					Weight: eval.HandlerWeight,
				{{end -}}
			{{else}}
				Weight: eval.FunctionWeight,
			{{end}}
		}, nil
	{{end}}
	}

	return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFields() []eval.Field {
	return []eval.Field{
		{{range $Name, $Field := .Fields}}
			"{{$Name}}",
		{{end}}
	}
}

func (e *Event) GetFieldValue(field eval.Field) (interface{}, error) {
	switch field {
		{{$Mock := .Mock}}
		{{range $Name, $Field := .Fields}}
		case "{{$Name}}":
		{{if $Field.Iterator}}
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
					result := {{$Return}}
				{{end}}

				values = append(values, result)

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
				return {{$Return}}, nil
			{{else if eq $Field.ReturnType "int"}}
				{{- if and ($Field.IsArray) (ne $Field.OrigType "int") }}
					result := make([]int, len({{$Return}}))
					for i, v := range {{$Return}} {
						result[i] = in(v)
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
		}

		return nil, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldEventType(field eval.Field) (eval.EventType, error) {
	switch field {
	{{range $Name, $Field := .Fields}}
	case "{{$Name}}":
		return "{{$Field.Event}}", nil
	{{end}}
	}

	return "", &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) GetFieldType(field eval.Field) (reflect.Kind, error) {
	switch field {
		{{range $Name, $Field := .Fields}}

		case "{{$Name}}":
		{{if eq $Field.ReturnType "string"}}
			return reflect.String, nil
		{{else if eq $Field.ReturnType "int"}}
			return reflect.Int, nil
		{{else if eq $Field.ReturnType "bool"}}
			return reflect.Bool, nil
		{{end}}
		{{end}}
		}

		return reflect.Invalid, &eval.ErrFieldNotFound{Field: field}
}

func (e *Event) SetFieldValue(field eval.Field, value interface{}) error {
	switch field {
		{{range $Name, $Field := .Fields}}
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
		{{else if eq $Field.BasicType "int"}}
			v, ok := value.(int)
			if !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}
			{{$FieldName}} = {{$Field.OrigType}}(v)
			return nil
		{{else if eq $Field.BasicType "bool"}}
			if {{$FieldName}}, ok = value.(bool); !ok {
				return &eval.ErrValueTypeMismatch{Field: "{{$Field.Name}}"}
			}
			return nil
		{{end}}
		{{end}}
		}

		return &eval.ErrFieldNotFound{Field: field}
}

`))

	os.Remove(output)

	module, err = parseFile(filename, pkgname)
	if err != nil {
		panic(err)
	}

	if genDoc {
		if err := genDocMain(module, output); err != nil {
			panic(err)
		}
		return
	}

	tmpfile, err := ioutil.TempFile(path.Dir(output), "accessors")
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
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
