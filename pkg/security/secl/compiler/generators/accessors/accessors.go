// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"bufio"
	"bytes"
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

	"github.com/Masterminds/sprig/v3"
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
	filename             string
	pkgname              string
	output               string
	verbose              bool
	mock                 bool
	docOutput            string
	fieldsResolverOutput string
	buildTags            string
)

var (
	packagesLookupMap map[string]*types.Package
)

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

func qualifiedType(module *common.Module, kind string) string {
	switch kind {
	case "int", "string", "bool":
		return kind
	default:
		return module.SourcePkgPrefix + kind
	}
}

// handleBasic adds fields of "basic" type to list of exposed SECL fields of the module
func handleBasic(module *common.Module, name, alias, kind, event string, iterator *common.StructField, isArray bool, opOverrides string, constants string, commentText string) *common.StructField {
	if verbose {
		fmt.Printf("handleBasic name: %s, kind: %s, alias: %s\n", name, kind, alias)
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
		Constants:   constants,
	}

	if _, ok := module.EventTypes[event]; !ok {
		module.EventTypes[event] = common.NewEventTypeMetada()
	}

	return module.Fields[alias]
}

func handleField(module *common.Module, astFile *ast.File, name, alias, prefix, aliasPrefix, pkgName string, fieldType string, event string, iterator *common.StructField, dejavu map[string]bool, isArray bool, opOverride string, constants string, commentText string, field seclField) error {
	if verbose {
		fmt.Printf("handleField fieldName %s, alias %s, prefix %s, aliasPrefix %s, pkgName %s, fieldType, %s\n", name, alias, prefix, aliasPrefix, pkgName, fieldType)
	}

	switch fieldType {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "net.IPNet":
		if prefix != "" {
			name = prefix + "." + name
			alias = aliasPrefix + "." + alias
		}
		handleBasic(module, name, alias, fieldType, event, iterator, isArray, opOverride, constants, commentText)
		if field.lengthField {
			field := handleBasic(module, name+".length", alias+".length", "int", event, iterator, isArray, opOverride, constants, commentText)
			field.IsLength = true
			field.OrigType = "int"
		}

	default:
		symbol, err := resolveSymbol(pkgName, fieldType)
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

		spec := astFile.Scope.Lookup(fieldType)
		handleSpec(module, astFile, spec.Decl, prefix, aliasPrefix, event, iterator, dejavu)
	}

	return nil
}

func getFieldName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return getFieldName(expr.X)
	case *ast.ArrayType:
		return getFieldName(expr.Elt)
	case *ast.SelectorExpr:
		return getFieldName(expr.X) + "." + getFieldName(expr.Sel)
	default:
		return ""
	}
}

func getFieldIdentName(expr ast.Expr) (name string, isPointer bool, isArray bool) {
	switch expr.(type) {
	case *ast.StarExpr:
		isPointer = true
	case *ast.ArrayType:
		isArray = true
	}

	return getFieldName(expr), isPointer, isArray
}

type seclField struct {
	name                   string
	iterator               string
	handler                string
	cachelessResolution    bool
	skipADResolution       bool
	lengthField            bool
	weight                 int64
	check                  string
	exposedAtEventRootOnly bool // fields that should only be exposed at the root of an event, i.e. `parent` should not be exposed for an `ancestor` of a process
}

func parseFieldDef(def string) (seclField, error) {
	def = strings.TrimSpace(def)
	alias, options, splitted := strings.Cut(def, ",")
	field := seclField{name: alias}

	if alias == "-" {
		return field, nil
	}

	// arguments
	if splitted {
		for _, el := range strings.Split(options, ",") {
			kv := strings.Split(el, ":")

			key, value := kv[0], kv[1]

			switch key {
			case "handler":
				field.handler = value
			case "weight":
				weight, err := strconv.ParseInt(value, 10, 64)
				if err != nil {
					return field, err
				}
				field.weight = weight
			case "iterator":
				field.iterator = value
			case "check":
				field.check = value
			case "opts":
				for _, opt := range strings.Split(value, "|") {
					switch opt {
					case "cacheless_resolution":
						field.cachelessResolution = true
					case "length":
						field.lengthField = true
					case "skip_ad":
						field.skipADResolution = true
					case "exposed_at_event_root_only":
						field.exposedAtEventRootOnly = true
					}
				}
			}
		}
	}

	return field, nil
}

// handleSpec is a recursive function that walks through the fields of a module
func handleSpec(module *common.Module, astFile *ast.File, spec interface{}, prefix, aliasPrefix, event string, iterator *common.StructField, dejavu map[string]bool) {
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
					if _, ok = module.EventTypes[e]; !ok {
						module.EventTypes[e] = common.NewEventTypeMetada()
						dejavu = make(map[string]bool) // clear dejavu map when it's a new event type
					}
					module.EventTypes[e].Doc = fieldCommentText
				}

				if isEmbedded := len(field.Names) == 0; !isEmbedded {
					fieldBasename := field.Names[0].Name
					if !unicode.IsUpper(rune(fieldBasename[0])) {
						continue
					}

					if dejavu[fieldBasename] {
						continue
					}

					var opOverrides string
					var constants string
					var fields []seclField
					fieldType, isPointer, isArray := getFieldIdentName(field.Type)

					if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
						for _, tag := range tags.Tags() {
							switch tag.Key {
							case "field":

								fieldDefs := strings.Split(tag.Value(), ";")
								for _, fieldDef := range fieldDefs {
									field, err := parseFieldDef(fieldDef)
									if err != nil {
										log.Panicf("unable to parse field definition: %s", err)
									}

									if field.name == "-" {
										continue FIELD
									}

									fields = append(fields, field)
								}

							case "op_override":
								opOverrides = tag.Value()
							case "constants":
								constants = tag.Value()
							}
						}
					} else {
						fields = append(fields, seclField{name: fieldBasename})
					}

					for _, seclField := range fields {
						fieldBasenameSECLNormalized := seclField.name
						alias := fieldBasenameSECLNormalized
						if aliasPrefix != "" {
							alias = aliasPrefix + "." + fieldBasenameSECLNormalized
						}

						prefixedFieldName := fmt.Sprintf("%s.%s", prefix, fieldBasename)
						if len(prefix) == 0 {
							prefixedFieldName = fieldBasename
						}

						// maintain a list of all the fields, including their prefixes
						module.AllFields[prefixedFieldName] = &common.StructField{
							Name:          prefixedFieldName,
							Event:         event,
							OrigType:      qualifiedType(module, fieldType),
							IsOrigTypePtr: isPointer,
							IsArray:       isArray,
							Constants:     constants,
							Check:         seclField.check,
						}

						if iterator := seclField.iterator; iterator != "" {
							module.Iterators[alias] = &common.StructField{
								Name:                prefixedFieldName,
								ReturnType:          qualifiedType(module, iterator),
								Event:               event,
								OrigType:            qualifiedType(module, fieldType),
								IsOrigTypePtr:       isPointer,
								IsArray:             isArray,
								Weight:              seclField.weight,
								CommentText:         fieldCommentText,
								OpOverrides:         opOverrides,
								Constants:           constants,
								CachelessResolution: seclField.cachelessResolution,
								SkipADResolution:    seclField.skipADResolution,
								Check:               seclField.check,
							}

							fieldIterator = module.Iterators[alias]
						}

						if handler := seclField.handler; handler != "" {
							if aliasPrefix != "" {
								fieldBasenameSECLNormalized = aliasPrefix + "." + fieldBasenameSECLNormalized
							}

							module.Fields[fieldBasenameSECLNormalized] = &common.StructField{
								Prefix:              prefix,
								Name:                prefixedFieldName,
								BasicType:           origTypeToBasicType(fieldType),
								Struct:              typeSpec.Name.Name,
								Handler:             handler,
								ReturnType:          origTypeToBasicType(fieldType),
								Event:               event,
								OrigType:            fieldType,
								Iterator:            fieldIterator,
								IsArray:             isArray,
								Weight:              seclField.weight,
								CommentText:         fieldCommentText,
								OpOverrides:         opOverrides,
								Constants:           constants,
								CachelessResolution: seclField.cachelessResolution,
								SkipADResolution:    seclField.skipADResolution,
								IsOrigTypePtr:       isPointer,
								Check:               seclField.check,
							}

							if seclField.lengthField {
								var lengthField common.StructField = *module.Fields[fieldBasenameSECLNormalized]
								lengthField.IsLength = true
								lengthField.Name += ".length"
								lengthField.OrigType = "int"
								lengthField.BasicType = "int"
								lengthField.ReturnType = "int"
								module.Fields[fieldBasenameSECLNormalized+".length"] = &lengthField
								lengthField.CommentText = "Length of '" + fieldBasenameSECLNormalized + "' string"
							}

							if _, ok = module.EventTypes[event]; !ok {
								module.EventTypes[event] = common.NewEventTypeMetada(fieldBasenameSECLNormalized)
							} else {
								module.EventTypes[event].Fields = append(module.EventTypes[event].Fields, fieldBasenameSECLNormalized)
							}
							delete(dejavu, fieldBasename)

							continue
						}

						dejavu[fieldBasename] = true

						if len(fieldType) != 0 {
							if err := handleField(module, astFile, fieldBasename, fieldBasenameSECLNormalized, prefix, aliasPrefix, pkgname, fieldType, event, fieldIterator, dejavu, false, opOverrides, constants, fieldCommentText, seclField); err != nil {
								log.Print(err)
							}

							if !seclField.exposedAtEventRootOnly {
								delete(dejavu, fieldBasename)
							}
						}

						if verbose {
							log.Printf("Don't know what to do with %s: %s", fieldBasename, spew.Sdump(field.Type))
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

							prefixedFieldName := fmt.Sprintf("%s.%s", prefix, ident.Name)
							if len(prefix) == 0 {
								prefixedFieldName = ident.Name
							}
							fieldType, isPointer, isArray := getFieldIdentName(field.Type)

							// maintain a list of all the fields
							module.AllFields[prefixedFieldName] = &common.StructField{
								Name:          prefixedFieldName,
								Event:         event,
								OrigType:      qualifiedType(module, fieldType),
								IsOrigTypePtr: isPointer,
								IsArray:       isArray,
							}

							handleSpec(module, astFile, embedded.Decl, prefix+"."+ident.Name, aliasPrefix, event, fieldIterator, dejavu)
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

	module := &common.Module{
		Name:       moduleName,
		SourcePkg:  pkgName,
		TargetPkg:  pkgName,
		BuildTags:  buildTags,
		Fields:     make(map[string]*common.StructField),
		AllFields:  make(map[string]*common.StructField),
		Iterators:  make(map[string]*common.StructField),
		EventTypes: make(map[string]*common.EventTypeMetadata),
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
					handleSpec(module, astFile, typeSpec, "", "", "", nil, make(map[string]bool))
				}
			}
		}
	}

	return module, nil
}

func newField(allFields map[string]*common.StructField, field *common.StructField) string {
	var path, result string
	for _, node := range strings.Split(field.Name, ".") {
		if path != "" {
			path += "." + node
		} else {
			path = node
		}

		if field, ok := allFields[path]; ok {
			if field.IsOrigTypePtr {
				result += fmt.Sprintf("if e.%s == nil { e.%s = &%s{} }\n", field.Name, field.Name, field.OrigType)
			}
		}
	}

	return result
}

func getFieldResolver(allFields map[string]*common.StructField, field *common.StructField) string {
	if field.Handler == "" || field.Iterator != nil || field.CachelessResolution {
		return ""
	}

	if field.Prefix == "" {
		return fmt.Sprintf("ev.%s(ev)", field.Handler)
	}

	ptr := "&"
	if allFields[field.Prefix].IsOrigTypePtr {
		ptr = ""
	}

	return fmt.Sprintf("ev.%s(%sev.%s)", field.Handler, ptr, field.Prefix)
}

func fieldADPrint(field *common.StructField, resolver string) string {
	if field.SkipADResolution {
		return fmt.Sprintf("if !forADs { _ = %s }", resolver)
	}
	return fmt.Sprintf("_ = %s", resolver)
}

func override(str string, mock bool) string {
	if !strings.Contains(str, ".") && !mock {
		return "model." + str
	}
	return str
}

func getHolder(allFields map[string]*common.StructField, field *common.StructField) *common.StructField {
	idx := strings.LastIndex(field.Name, ".")
	if idx == -1 {
		return nil
	}
	name := field.Name[:idx]
	return allFields[name]
}

func getChecks(allFields map[string]*common.StructField, field *common.StructField) []string {
	var checks []string

	name := field.Name
	for name != "" {
		field := allFields[name]
		if field == nil {
			break
		}

		if field.Check != "" {
			if holder := getHolder(allFields, field); holder != nil {
				check := fmt.Sprintf(`%s.%s`, holder.Name, field.Check)
				checks = append([]string{check}, checks...)
			}
		}

		idx := strings.LastIndex(name, ".")
		if idx == -1 {
			break
		}
		name = name[:idx]
	}

	return checks
}

var funcMap = map[string]interface{}{
	"TrimPrefix":       strings.TrimPrefix,
	"TrimSuffix":       strings.TrimSuffix,
	"HasPrefix":        strings.HasPrefix,
	"NewField":         newField,
	"Override":         override,
	"GetFieldResolver": getFieldResolver,
	"FieldADPrint":     fieldADPrint,
	"GetChecks":        getChecks,
}

//go:embed accessors.tmpl
var accessorsTemplateCode string

//go:embed fields_resolver.tmpl
var fieldsResolverTemplate string

func main() {
	module, err := parseFile(filename, pkgname)
	if err != nil {
		panic(err)
	}

	if len(fieldsResolverOutput) > 0 {
		if err = GenerateContent(fieldsResolverOutput, module, fieldsResolverTemplate); err != nil {
			panic(err)
		}
	}

	if docOutput != "" {
		os.Remove(docOutput)
		if err := doc.GenerateDocJSON(module, path.Dir(filename), docOutput); err != nil {
			panic(err)
		}
	}

	os.Remove(output)
	if err := GenerateContent(output, module, accessorsTemplateCode); err != nil {
		panic(err)
	}
}

// GenerateContent generates with the given template
func GenerateContent(output string, module *common.Module, tmplCode string) error {
	tmpl := template.Must(template.New("header").Funcs(funcMap).Funcs(sprig.TxtFuncMap()).Parse(tmplCode))

	buffer := bytes.Buffer{}
	if err := tmpl.Execute(&buffer, module); err != nil {
		return err
	}

	cleaned := removeEmptyLines(&buffer)

	tmpfile, err := os.CreateTemp(path.Dir(output), "secl-helpers")
	if err != nil {
		return err
	}

	if _, err := tmpfile.WriteString(cleaned); err != nil {
		return err
	}

	if err := tmpfile.Close(); err != nil {
		return err
	}

	cmd := exec.Command("gofmt", "-s", "-w", tmpfile.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		log.Fatal(string(output))
		return err
	}

	return os.Rename(tmpfile.Name(), output)
}

func removeEmptyLines(input *bytes.Buffer) string {
	scanner := bufio.NewScanner(input)
	builder := strings.Builder{}
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if len(trimmed) != 0 {
			builder.WriteString(trimmed)
			builder.WriteRune('\n')
		}
	}
	return builder.String()
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Be verbose")
	flag.BoolVar(&mock, "mock", false, "Mock accessors")
	flag.StringVar(&docOutput, "doc", "", "Generate documentation JSON")
	flag.StringVar(&fieldsResolverOutput, "fields-resolver", "", "Fields resolver output file")
	flag.StringVar(&filename, "input", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
