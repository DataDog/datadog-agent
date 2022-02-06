// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	_ "embed"
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
	pkgname           string
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

func origTypeToBasicType(kind string) string {
	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "int"
	}
	return kind
}

func handleBasic(name, alias, kind, event string, iterator *common.StructField, isArray bool, opOverrides string, commentText string) {
	if verbose {
		fmt.Printf("handleBasic %s %s\n", name, kind)
	}

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
		OpOverrides: opOverrides,
	}

	module.EventTypes[event] = true
}

func handleField(astFile *ast.File, name, alias, prefix, aliasPrefix, pkgName string, fieldType *ast.Ident, event string, iterator *common.StructField, dejavu map[string]bool, isArray bool, opOverride string, commentText string) error {
	if verbose {
		fmt.Printf("handleField fieldName %s, alias %s, prefix %s, aliasPrefix %s, pkgName %s, fieldType, %s\n", name, alias, prefix, aliasPrefix, pkgName, fieldType)
	}

	switch fieldType.Name {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}
		handleBasic(name, alias, fieldType.Name, event, iterator, isArray, opOverride, commentText)

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
	if verbose {
		fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s, event %s, iterator %+v\n", spec, prefix, aliasPrefix, event, iterator)
	}

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

					var opOverrides string
					var fields []seclField
					fieldType, isPointer, isArray := getFieldIdent(field)

					var weight int64
					if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
						for _, tag := range tags.Tags() {
							switch tag.Key {
							case "field":
								splitted := strings.SplitN(tag.Value(), ",", 3)
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
							case "op_override":
								opOverrides = tag.Value()
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
								OpOverrides:   opOverrides,
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
								OpOverrides: opOverrides,
							}

							module.EventTypes[event] = true
							delete(dejavu, fieldName)

							continue
						}

						dejavu[fieldName] = true

						if fieldType != nil {
							if err := handleField(astFile, fieldName, fieldAlias, prefix, aliasPrefix, pkgname, fieldType, event, fieldIterator, dejavu, false, opOverrides, fieldCommentText); err != nil {
								log.Print(err)
							}

							delete(dejavu, fieldName)
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
							if verbose {
								log.Printf("Embedded struct %s", ident.Name)
							}
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
					if strings.Contains(doc.Text, "genaccessors") {
						genaccessors = true
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

//go:embed accessors.tmpl
var accessorsTemplateCode string

func main() {
	var err error
	tmpl := template.Must(template.New("header").Funcs(FuncMap).Parse(accessorsTemplateCode))

	os.Remove(output)

	module, err = parseFile(filename, pkgname)
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
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
