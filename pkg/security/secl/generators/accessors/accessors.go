// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
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
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
	"unicode"

	"github.com/davecgh/go-spew/spew"
	"golang.org/x/tools/go/loader"
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
	program   *loader.Program
	packages  map[string]*types.Package
	buildTags string
)

type Module struct {
	Name            string
	SourcePkgPrefix string
	SourcePkg       string
	TargetPkg       string
	BuildTags       []string
	Fields          map[string]*structField
	Iterators       map[string]*structField
	EventTypes      map[string]bool
	Mock            bool
}

var module *Module

type structField struct {
	Name          string
	Prefix        string
	Struct        string
	BasicType     string
	ReturnType    string
	IsArray       bool
	Public        bool
	Event         string
	Handler       string
	OrigType      string
	IsOrigTypePtr bool
	Iterator      *structField
}

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

func handleBasic(name, alias, kind, event string, iterator *structField, isArray bool) {
	fmt.Printf("handleBasic %s %s\n", name, kind)

	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		module.Fields[alias] = &structField{Name: name, ReturnType: "int", Public: true, Event: event, OrigType: kind, BasicType: origTypeToBasicType(kind), Iterator: iterator}
		module.EventTypes[event] = true
	default:
		public := false
		firstChar := strings.TrimPrefix(kind, "[]")
		if splits := strings.Split(firstChar, "."); len(splits) > 1 {
			firstChar = splits[len(splits)-1]
		}
		if unicode.IsUpper(rune(firstChar[0])) {
			public = true
		}
		module.Fields[alias] = &structField{
			Name:       name,
			BasicType:  origTypeToBasicType(kind),
			ReturnType: kind,
			IsArray:    strings.HasPrefix(kind, "[]") || isArray,
			Public:     public,
			Event:      event,
			OrigType:   kind,
			Iterator:   iterator,
		}
		module.EventTypes[event] = true
	}
}

func handleField(astFile *ast.File, name, alias, prefix, aliasPrefix, pkgName string, fieldType *ast.Ident, event string, iterator *structField, dejavu map[string]bool, isArray bool) error {
	fmt.Printf("handleField fieldName %s, alias %s, prefix %s, aliasPrefix %s, pkgName %s, fieldType, %s\n", name, alias, prefix, aliasPrefix, pkgName, fieldType)

	switch fieldType.Name {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}
		handleBasic(name, alias, fieldType.Name, event, iterator, isArray)
	default:
		symbol, err := resolveSymbol(pkgName, fieldType.Name)
		if err != nil {
			return fmt.Errorf("Failed to resolve symbol for %+v in %s: %s", fieldType, pkgName, err)
		}
		if symbol == nil {
			return fmt.Errorf("Failed to resolve symbol for %+v in %s", fieldType, pkgName)
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

func handleSpec(astFile *ast.File, spec interface{}, prefix, aliasPrefix, event string, iterator *structField, dejavu map[string]bool) {
	fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s, event %s, iterator %+v\n", spec, prefix, aliasPrefix, event, iterator)

	if typeSpec, ok := spec.(*ast.TypeSpec); ok {
		if structType, ok := typeSpec.Type.(*ast.StructType); ok {
		FIELD:
			for _, field := range structType.Fields.List {
				fieldIterator := iterator

				var tag reflect.StructTag
				if field.Tag != nil {
					tag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
				}

				if e, ok := tag.Lookup("event"); ok {
					event = e
				}

				if isEmbedded := len(field.Names) == 0; !isEmbedded {
					fieldName := field.Names[0].Name
					fieldAlias := fieldName

					if dejavu[fieldName] {
						continue
					}

					if fieldTag, found := tag.Lookup("field"); found {
						if fieldAlias = fieldTag; fieldAlias == "-" {
							continue FIELD
						}

						if it, found := tag.Lookup("iterator"); found {
							alias := fieldAlias
							if aliasPrefix != "" {
								alias = aliasPrefix + "." + fieldAlias
							}

							var OrigType string
							var IsOrigTypePtr bool
							var IsArray bool

							if ft, ok := field.Type.(*ast.Ident); ok {
								OrigType = ft.Name
							} else if ft, ok := field.Type.(*ast.StarExpr); ok {
								if ident, ok := ft.X.(*ast.Ident); ok {
									OrigType = ident.Name
									IsOrigTypePtr = true
								}
							} else if ft, ok := field.Type.(*ast.ArrayType); ok {
								IsArray = true
								if ident, ok := ft.Elt.(*ast.Ident); ok {
									OrigType = ident.Name
								}
							}

							pkgType := func(t string) string {
								switch t {
								case "int", "string", "bool":
									return t
								default:
									return module.SourcePkgPrefix + t
								}
							}

							module.Iterators[alias] = &structField{
								Name:          fmt.Sprintf("%s.%s", prefix, fieldName),
								ReturnType:    pkgType(it),
								Public:        true,
								Event:         event,
								OrigType:      pkgType(OrigType),
								IsOrigTypePtr: IsOrigTypePtr,
								IsArray:       IsArray,
							}

							fieldIterator = module.Iterators[alias]
						}

						if handler, found := tag.Lookup("handler"); found {
							els := strings.Split(handler, ",")
							if len(els) != 2 {
								panic("handler definition should be `FunctionName,ReturnType`")
							}
							fnc, kind := els[0], els[1]

							if aliasPrefix != "" {
								fieldAlias = aliasPrefix + "." + fieldAlias
							}

							fieldType, ok := field.Type.(*ast.Ident)
							if ok {
								module.Fields[fieldAlias] = &structField{
									Prefix:     prefix,
									Name:       fmt.Sprintf("%s.%s", prefix, fieldName),
									BasicType:  origTypeToBasicType(fieldType.Name),
									Struct:     typeSpec.Name.Name,
									Handler:    fnc,
									ReturnType: kind,
									Public:     true,
									Event:      event,
									OrigType:   fieldType.Name,
									Iterator:   fieldIterator,
								}
								module.EventTypes[event] = true
							}
							delete(dejavu, fieldName)

							continue
						}
					}

					dejavu[fieldName] = true

					if fieldType, ok := field.Type.(*ast.Ident); ok {
						if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, filepath.Base(pkgname), fieldType, event, fieldIterator, dejavu, false); err != nil {
							log.Print(err)
						}
						delete(dejavu, fieldName)

						continue
					} else if fieldType, ok := field.Type.(*ast.StarExpr); ok {
						if ident, ok := fieldType.X.(*ast.Ident); ok {
							if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, filepath.Base(pkgname), ident, event, fieldIterator, dejavu, false); err != nil {
								log.Print(err)
							}
							delete(dejavu, fieldName)

							continue
						}
					} else if ft, ok := field.Type.(*ast.ArrayType); ok {
						if ident, ok := ft.Elt.(*ast.Ident); ok {
							if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, filepath.Base(pkgname), ident, event, fieldIterator, dejavu, true); err != nil {
								log.Print(err)
							}

							delete(dejavu, fieldName)

							continue
						}
					}

					delete(dejavu, fieldName)

					if strict {
						log.Panicf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
					}
					if verbose {
						log.Printf("Don't know what to do with %s: %s", fieldName, spew.Sdump(field.Type))
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

func parseFile(filename string, pkgName string) (*Module, error) {
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
		packages[typePackage.Name()] = typePackage
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

	module = &Module{
		Name:       moduleName,
		SourcePkg:  pkgName,
		TargetPkg:  pkgName,
		BuildTags:  buildTags,
		Fields:     make(map[string]*structField),
		Iterators:  make(map[string]*structField),
		EventTypes: make(map[string]bool),
		Mock:       mock,
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
	{{if $Field.Iterator}}
		{{$EvaluatorType = "eval.StringArrayEvaluator"}}
	{{end}}
	{{if eq $Field.ReturnType "int"}}
		{{$EvaluatorType = "eval.IntEvaluator"}}
		{{if $Field.Iterator}}
			{{$EvaluatorType = "eval.IntArrayEvaluator"}}
		{{end}}
	{{else if eq $Field.ReturnType "bool"}}
		{{$EvaluatorType = "eval.BoolEvaluator"}}
		{{if $Field.Iterator}}
			{{$EvaluatorType = "eval.BoolArrayEvaluator"}}
		{{end}}
	{{end}}

	case "{{$Name}}":
		return &{{$EvaluatorType}}{
			{{- if $Field.Iterator}}
				EvalFnc: func(ctx *eval.Context) []{{$Field.ReturnType}} {
					if ptr := ctx.Cache[field]; ptr != nil {
						if result := (*[]{{$Field.ReturnType}})(ptr); result != nil {
							return *result
						}
					}

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

					ctx.Cache[field] = unsafe.Pointer(&results)

					return results
				},
			{{- else}}
				EvalFnc: func(ctx *eval.Context) {{$Field.ReturnType}} {
					{{$Return := $Field.Name | printf "(*Event)(ctx.Object).%s"}}
					{{if and (ne $Field.Handler "") (not $Mock)}}
						{{$Return = print "(*Event)(ctx.Object)." $Field.Handler "(&(*Event)(ctx.Object)." $Field.Prefix ")"}}
					{{end}}

					{{- if eq $Field.ReturnType "int"}}
						return int({{$Return}})
					{{- else}}
						return {{$Return}}
					{{end -}}
				},
			{{end -}}
			Field: field,
			{{if $Field.Iterator}}
				Weight: eval.IteratorWeight,
			{{else if $Field.Handler}}
				Weight: eval.HandlerWeight,
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

				{{if eq $Field.ReturnType "int"}}
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

			{{if eq $Field.ReturnType "string"}}
				return {{$Return}}, nil
			{{else if eq $Field.ReturnType "int"}}
				return int({{$Return}}), nil
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
	flag.StringVar(&filename, "input", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
