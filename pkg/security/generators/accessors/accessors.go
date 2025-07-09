// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main holds main related files
package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"log"
	"os"
	"os/exec"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/Masterminds/sprig/v3"
	"github.com/davecgh/go-spew/spew"
	"github.com/fatih/structtag"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/tools/go/packages"

	"github.com/DataDog/datadog-agent/pkg/security/generators/accessors/common"
	"github.com/DataDog/datadog-agent/pkg/security/generators/accessors/doc"
)

const (
	pkgPrefix = "github.com/DataDog/datadog-agent/pkg/security/secl"
)

var (
	modelFile            string
	typesFile            string
	pkgname              string
	output               string
	verbose              bool
	docOutput            string
	fieldHandlersOutput  string
	fieldAccessorsOutput string
	buildTags            string
)

// AstFiles defines ast files
type AstFiles struct {
	files []*ast.File
}

// LookupSymbol lookups symbol
func (af *AstFiles) LookupSymbol(symbol string) *ast.Object { //nolint:staticcheck
	for _, file := range af.files {
		if obj := file.Scope.Lookup(symbol); obj != nil {
			return obj
		}
	}
	return nil
}

// Parse extract data
func (af *AstFiles) Parse() ([]ast.Spec, []string) {
	var (
		specs   []ast.Spec
		getters []string
	)

	for _, file := range af.files {
		for _, decl := range file.Decls {
			decl, ok := decl.(*ast.GenDecl)
			if !ok || decl.Doc == nil {
				continue
			}

			var genaccessors bool
			for _, document := range decl.Doc.List {
				if strings.Contains(document.Text, "genaccessors") {
					genaccessors = true
				}

				if strings.Contains(document.Text, "gengetter") {
					els := strings.Split(document.Text, ":")
					if len(els) > 1 {
						getters = append(getters, strings.TrimSpace(els[1]))
					}
				}
			}

			if !genaccessors {
				continue
			}

			specs = append(specs, decl.Specs...)
		}
	}

	return specs, getters
}

func origTypeToBasicType(kind string) string {
	switch kind {
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64":
		return "int"
	case "containerutils.ContainerID", "containerutils.CGroupID":
		return "string"
	}
	return kind
}

func isBasicType(kind string) bool {
	switch kind {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "net.IPNet":
		return true
	}
	return false
}

func isBasicTypeForGettersOnly(kind string) bool {
	if isBasicType(kind) {
		return true
	}

	switch kind {
	case "time.Time":
		return true
	}
	return false
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
func handleBasic(module *common.Module, field seclField, name, alias, aliasPrefix, prefix, kind, event string, restrictedTo []string, opOverrides, commentText, containerStructName string, iterator *common.StructField, isArray bool) {
	if verbose {
		fmt.Printf("handleBasic name: %s, kind: %s, alias: %s, isArray: %v\n", name, kind, alias, isArray)
	}

	if prefix != "" {
		name = prefix + "." + name
	}

	if aliasPrefix != "" {
		alias = aliasPrefix + "." + alias
	}

	basicType := origTypeToBasicType(kind)
	newStructField := &common.StructField{
		Name:         name,
		BasicType:    basicType,
		ReturnType:   basicType,
		IsArray:      strings.HasPrefix(kind, "[]") || isArray,
		Event:        event,
		OrigType:     kind,
		Iterator:     iterator,
		CommentText:  commentText,
		OpOverrides:  opOverrides,
		Struct:       containerStructName,
		Alias:        alias,
		AliasPrefix:  aliasPrefix,
		GettersOnly:  field.gettersOnly,
		Ref:          field.ref,
		RestrictedTo: restrictedTo,
	}

	module.Fields[alias] = newStructField

	if _, ok := module.EventTypes[event]; !ok {
		module.EventTypes[event] = common.NewEventTypeMetada()
	}

	if field.lengthField {
		name = name + ".length"
		aliasPrefix = alias
		alias = alias + ".length"

		newStructField := &common.StructField{
			Name:         name,
			BasicType:    "int",
			ReturnType:   "int",
			OrigType:     "int",
			IsArray:      isArray,
			IsLength:     true,
			Event:        event,
			Iterator:     iterator,
			CommentText:  doc.SECLDocForLength,
			OpOverrides:  opOverrides,
			Struct:       "string",
			Alias:        alias,
			AliasPrefix:  aliasPrefix,
			GettersOnly:  field.gettersOnly,
			Ref:          field.ref,
			RestrictedTo: restrictedTo,
		}

		module.Fields[alias] = newStructField
	}

	if _, ok := module.EventTypes[event]; !ok {
		module.EventTypes[event] = common.NewEventTypeMetada(alias)
	} else {
		module.EventTypes[event].Fields = append(module.EventTypes[event].Fields, alias)
	}
}

// handleEmbedded adds embedded fields to list of exposed SECL fields of the module
func handleEmbedded(module *common.Module, name, prefix, event string, restrictedTo []string, fieldTypeExpr ast.Expr) {
	if verbose {
		log.Printf("handleEmbedded name: %s", name)
	}

	if prefix != "" {
		name = fmt.Sprintf("%s.%s", prefix, name)
	}

	fieldType, isPointer, isArray := getFieldIdentName(fieldTypeExpr)

	// maintain a list of all the fields
	module.AllFields[name] = &common.StructField{
		Name:          name,
		Event:         event,
		OrigType:      qualifiedType(module, fieldType),
		IsOrigTypePtr: isPointer,
		IsArray:       isArray,
		RestrictedTo:  restrictedTo,
	}
}

// handleNonEmbedded adds non-embedded fields to list of all possible (but not necessarily exposed) SECL fields of the module
func handleNonEmbedded(module *common.Module, field seclField, aliasPrefix, alias, prefixedFieldName, event string, restrictedTo []string, fieldType string, isPointer, isArray bool) {
	if aliasPrefix != "" {
		alias = aliasPrefix + "." + alias
	}

	module.AllFields[prefixedFieldName] = &common.StructField{
		Name:          prefixedFieldName,
		Event:         event,
		OrigType:      qualifiedType(module, fieldType),
		IsOrigTypePtr: isPointer,
		IsArray:       isArray,
		Check:         field.check,
		RestrictedTo:  restrictedTo,
		SetHandler:    field.setHandler,
		AliasPrefix:   aliasPrefix,
		Alias:         alias,
	}
}

func addLengthOpField(module *common.Module, alias string, field *common.StructField) *common.StructField {
	lengthField := *field
	lengthField.IsLength = true
	lengthField.Name += ".length"
	lengthField.OrigType = "int"
	lengthField.BasicType = "int"
	lengthField.ReturnType = "int"
	lengthField.Struct = "string"
	lengthField.AliasPrefix = alias
	lengthField.Alias = alias + ".length"
	lengthField.CommentText = doc.SECLDocForLength

	module.Fields[lengthField.Alias] = &lengthField

	return &lengthField
}

// handleIterator adds iterator to list of exposed SECL iterators of the module
func handleIterator(module *common.Module, field seclField, fieldType, iterator, aliasPrefix, prefixedFieldName, event string, restrictedTo []string, fieldCommentText, opOverrides string, isPointer, isArray bool) *common.StructField {
	alias := field.name
	if aliasPrefix != "" {
		alias = aliasPrefix + "." + field.name
	}

	module.Iterators[alias] = &common.StructField{
		Name:             prefixedFieldName,
		ReturnType:       qualifiedType(module, iterator),
		Event:            event,
		OrigType:         qualifiedType(module, fieldType),
		IsOrigTypePtr:    isPointer,
		IsArray:          isArray,
		Weight:           field.weight,
		CommentText:      fieldCommentText,
		OpOverrides:      opOverrides,
		Helper:           field.helper,
		SkipADResolution: field.skipADResolution,
		Check:            field.check,
		SetHandler:       field.setHandler,
		Ref:              field.ref,
		RestrictedTo:     restrictedTo,
		ReadOnly:         field.readOnly,
	}

	lengthField := addLengthOpField(module, alias, module.Iterators[alias])
	lengthField.Iterator = module.Iterators[alias]
	lengthField.IsIterator = true

	return module.Iterators[alias]
}

// handleFieldWithHandler adds non-embedded fields with handlers to list of exposed SECL fields and event types of the module
func handleFieldWithHandler(module *common.Module, field seclField, aliasPrefix, prefix, prefixedFieldName, fieldType, containerStructName, event string, restrictedTo []string, fieldCommentText, opOverrides, handler string, isPointer, isArray bool, fieldIterator *common.StructField) {
	alias := field.name

	if aliasPrefix != "" {
		alias = aliasPrefix + "." + alias
	}

	if event == "" && verbose {
		log.Printf("event type not specified for field: %s", prefixedFieldName)
	}

	newStructField := &common.StructField{
		Prefix:           prefix,
		Name:             prefixedFieldName,
		BasicType:        origTypeToBasicType(fieldType),
		Struct:           containerStructName,
		Handler:          handler,
		ReturnType:       origTypeToBasicType(fieldType),
		Event:            event,
		OrigType:         fieldType,
		Iterator:         fieldIterator,
		IsArray:          isArray,
		Weight:           field.weight,
		CommentText:      fieldCommentText,
		OpOverrides:      opOverrides,
		Helper:           field.helper,
		SkipADResolution: field.skipADResolution,
		IsOrigTypePtr:    isPointer,
		Check:            field.check,
		SetHandler:       field.setHandler,
		Alias:            alias,
		AliasPrefix:      aliasPrefix,
		GettersOnly:      field.gettersOnly,
		Ref:              field.ref,
		RestrictedTo:     restrictedTo,
		ReadOnly:         field.readOnly,
	}
	module.Fields[alias] = newStructField

	if field.lengthField {
		addLengthOpField(module, alias, module.Fields[alias])
	}

	if _, ok := module.EventTypes[event]; !ok {
		module.EventTypes[event] = common.NewEventTypeMetada(alias)
	} else {
		module.EventTypes[event].Fields = append(module.EventTypes[event].Fields, alias)
	}
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
	helper                 bool // mark the handler as just a helper and not a real resolver. Won't be called by ResolveFields
	skipADResolution       bool
	lengthField            bool
	weight                 int64
	check                  string
	setHandler             string
	exposedAtEventRootOnly bool // fields that should only be exposed at the root of an event, i.e. `parent` should not be exposed for an `ancestor` of a process
	containerStructName    string
	gettersOnly            bool //  a field that is not exposed via SECL, but still has an accessor generated
	ref                    string
	readOnly               bool
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
			case "ref":
				field.ref = value
			case "iterator":
				field.iterator = value
			case "check":
				field.check = value
			case "set_handler":
				field.setHandler = value
			case "opts":
				for _, opt := range strings.Split(value, "|") {
					switch opt {
					case "helper":
						field.helper = true
					case "length":
						field.lengthField = true
					case "skip_ad":
						field.skipADResolution = true
					case "exposed_at_event_root_only":
						field.exposedAtEventRootOnly = true
					case "getters_only":
						field.gettersOnly = true
						field.exposedAtEventRootOnly = true
					case "readonly":
						field.readOnly = true
					}
				}
			}
		}
	}

	return field, nil
}

// handleSpecRecursive is a recursive function that walks through the fields of a module
func handleSpecRecursive(module *common.Module, astFiles *AstFiles, spec interface{}, prefix, aliasPrefix, event string, restrictedTo []string, iterator *common.StructField, dejavu map[string]bool) {
	if verbose {
		fmt.Printf("handleSpec spec: %+v, prefix: %s, aliasPrefix %s, event %s, iterator %+v\n", spec, prefix, aliasPrefix, event, iterator)
	}

	var typeSpec *ast.TypeSpec
	var structType *ast.StructType
	var ok bool
	if typeSpec, ok = spec.(*ast.TypeSpec); !ok {
		return
	}
	if structType, ok = typeSpec.Type.(*ast.StructType); !ok {
		log.Printf("Don't know what to do with %s (%s)", typeSpec.Name, spew.Sdump(typeSpec))
		return
	}

	prevrestrictedTo := restrictedTo

	for _, field := range structType.Fields.List {
		fieldCommentText := field.Comment.Text()
		fieldIterator := iterator
		fieldEvent := event

		var tag reflect.StructTag
		if field.Tag != nil {
			tag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
		}

		if e, ok := tag.Lookup("event"); ok {
			fieldEvent = e
			if _, ok = module.EventTypes[e]; !ok {
				module.EventTypes[e] = common.NewEventTypeMetada()
				dejavu = make(map[string]bool) // clear dejavu map when it's a new event type
			}
			module.EventTypes[e].Doc = fieldCommentText
		}

		if e, ok := tag.Lookup("restricted_to"); ok {
			restrictedTo = strings.Split(e, ",")
		}

		if isEmbedded := len(field.Names) == 0; isEmbedded { // embedded as in a struct embedded in another struct
			if fieldTag, found := tag.Lookup("field"); found && fieldTag == "-" {
				continue
			}

			ident, _ := field.Type.(*ast.Ident)
			if ident == nil {
				if starExpr, ok := field.Type.(*ast.StarExpr); ok {
					ident, _ = starExpr.X.(*ast.Ident)
				}
			}

			if ident != nil {
				name := ident.Name
				if prefix != "" {
					name = prefix + "." + ident.Name
				}

				embedded := astFiles.LookupSymbol(ident.Name)
				if embedded != nil {
					handleEmbedded(module, ident.Name, prefix, fieldEvent, restrictedTo, field.Type)
					handleSpecRecursive(module, astFiles, embedded.Decl, name, aliasPrefix, fieldEvent, restrictedTo, fieldIterator, dejavu)
				} else {
					log.Printf("failed to resolve symbol for identifier %+v in %s", ident.Name, pkgname)
				}
			}
		} else {
			fieldBasename := field.Names[0].Name
			if !unicode.IsUpper(rune(fieldBasename[0])) {
				continue
			}

			if dejavu[fieldBasename] {
				continue
			}

			var opOverrides string
			var fields []seclField
			var gettersOnlyFields []seclField
			if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
				opOverrides, fields, gettersOnlyFields = parseTags(tags, typeSpec.Name.Name)

				if opOverrides == "" && fields == nil && gettersOnlyFields == nil {
					continue
				}
			} else {
				fields = append(fields, seclField{name: fieldBasename})
			}

			fieldType, isPointer, isArray := getFieldIdentName(field.Type)

			prefixedFieldName := fieldBasename
			if prefix != "" {
				prefixedFieldName = fmt.Sprintf("%s.%s", prefix, fieldBasename)
			}

			for _, seclField := range fields {
				alias := seclField.name

				handleNonEmbedded(module, seclField, aliasPrefix, alias, prefixedFieldName, fieldEvent, restrictedTo, fieldType, isPointer, isArray)

				if seclFieldIterator := seclField.iterator; seclFieldIterator != "" {
					fieldIterator = handleIterator(module, seclField, fieldType, seclFieldIterator, aliasPrefix, prefixedFieldName, fieldEvent, restrictedTo, fieldCommentText, opOverrides, isPointer, isArray)
				}

				if handler := seclField.handler; handler != "" {
					handleFieldWithHandler(module, seclField, aliasPrefix, prefix, prefixedFieldName, fieldType, seclField.containerStructName, fieldEvent, restrictedTo, fieldCommentText, opOverrides, handler, isPointer, isArray, fieldIterator)

					delete(dejavu, fieldBasename)
					continue
				}

				if verbose {
					log.Printf("Don't know what to do with %s: %s", fieldBasename, spew.Sdump(field.Type))
				}

				dejavu[fieldBasename] = true

				if len(fieldType) == 0 {
					continue
				}

				if isBasicType(fieldType) {
					handleBasic(module, seclField, fieldBasename, alias, aliasPrefix, prefix, fieldType, fieldEvent, restrictedTo, opOverrides, fieldCommentText, seclField.containerStructName, fieldIterator, isArray)
				} else {
					spec := astFiles.LookupSymbol(fieldType)
					if spec != nil {
						newPrefix, newAliasPrefix := fieldBasename, alias

						if prefix != "" {
							newPrefix = prefix + "." + fieldBasename
						}

						if aliasPrefix != "" {
							newAliasPrefix = aliasPrefix + "." + alias
						}

						handleSpecRecursive(module, astFiles, spec.Decl, newPrefix, newAliasPrefix, fieldEvent, restrictedTo, fieldIterator, dejavu)
					} else {
						log.Printf("failed to resolve symbol for type %+v in %s", fieldType, pkgname)
					}
				}

				if !seclField.exposedAtEventRootOnly {
					delete(dejavu, fieldBasename)
				}
			}
			for _, seclField := range gettersOnlyFields {
				alias := seclField.name

				handleNonEmbedded(module, seclField, aliasPrefix, alias, prefixedFieldName, fieldEvent, restrictedTo, fieldType, isPointer, isArray)

				if seclFieldIterator := seclField.iterator; seclFieldIterator != "" {
					fieldIterator = handleIterator(module, seclField, fieldType, seclFieldIterator, aliasPrefix, prefixedFieldName, fieldEvent, restrictedTo, fieldCommentText, opOverrides, isPointer, isArray)
				}

				if handler := seclField.handler; handler != "" {
					handleFieldWithHandler(module, seclField, aliasPrefix, prefix, prefixedFieldName, fieldType, seclField.containerStructName, fieldEvent, restrictedTo, fieldCommentText, opOverrides, handler, isPointer, isArray, fieldIterator)

					delete(dejavu, fieldBasename)
					continue
				}

				if verbose {
					log.Printf("Don't know what to do with %s: %s", fieldBasename, spew.Sdump(field.Type))
				}

				dejavu[fieldBasename] = true

				if len(fieldType) == 0 {
					continue
				}

				if isBasicTypeForGettersOnly(fieldType) {
					handleBasic(module, seclField, fieldBasename, alias, aliasPrefix, prefix, fieldType, fieldEvent, restrictedTo, opOverrides, fieldCommentText, seclField.containerStructName, fieldIterator, isArray)
				} else {
					spec := astFiles.LookupSymbol(fieldType)
					if spec != nil {
						newPrefix, newAliasPrefix := fieldBasename, alias

						if prefix != "" {
							newPrefix = prefix + "." + fieldBasename
						}

						if aliasPrefix != "" {
							newAliasPrefix = aliasPrefix + "." + alias
						}

						handleSpecRecursive(module, astFiles, spec.Decl, newPrefix, newAliasPrefix, fieldEvent, restrictedTo, fieldIterator, dejavu)
					} else {
						log.Printf("failed to resolve symbol for type %+v in %s", fieldType, pkgname)
					}
				}

				if !seclField.exposedAtEventRootOnly {
					delete(dejavu, fieldBasename)
				}
			}
		}

		restrictedTo = prevrestrictedTo
	}
}

func parseTags(tags *structtag.Tags, containerStructName string) (string, []seclField, []seclField) {
	var opOverrides string
	var fields []seclField
	var gettersOnlyFields []seclField

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
					return "", nil, nil
				}

				field.containerStructName = containerStructName

				if field.gettersOnly {
					gettersOnlyFields = append(gettersOnlyFields, field)
				} else {
					fields = append(fields, field)
				}
			}

		case "op_override":
			opOverrides = tag.Value()
		}
	}

	return opOverrides, fields, gettersOnlyFields
}

func newAstFiles(cfg *packages.Config, files ...string) (*AstFiles, error) {
	var astFiles AstFiles

	for _, file := range files {
		pkgs, err := packages.Load(cfg, file)
		if err != nil {
			return nil, err
		}

		if len(pkgs) == 0 || len(pkgs[0].Syntax) == 0 {
			return nil, fmt.Errorf("failed to get syntax from parse file %s", file)
		}

		astFiles.files = append(astFiles.files, pkgs[0].Syntax[0])
	}

	return &astFiles, nil
}

func parseFile(modelFile string, typesFile string, pkgName string) (*common.Module, error) {
	cfg := packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		BuildFlags: []string{"-mod=readonly", fmt.Sprintf("-tags=%s", buildTags)},
	}

	astFiles, err := newAstFiles(&cfg, modelFile, typesFile)
	if err != nil {
		return nil, err
	}

	moduleName := path.Base(path.Dir(output))
	if moduleName == "." {
		moduleName = path.Base(pkgName)
	}

	module := &common.Module{
		Name:       moduleName,
		SourcePkg:  pkgName,
		TargetPkg:  pkgName,
		BuildTags:  formatBuildTags(buildTags),
		Fields:     make(map[string]*common.StructField),
		AllFields:  make(map[string]*common.StructField),
		Iterators:  make(map[string]*common.StructField),
		EventTypes: make(map[string]*common.EventTypeMetadata),
	}

	// If the target package is different from the model package
	if module.Name != path.Base(pkgName) {
		module.SourcePkgPrefix = path.Base(pkgName) + "."
		module.TargetPkg = path.Clean(path.Join(pkgName, path.Dir(output)))
	}

	specs, getters := astFiles.Parse()
	module.Getters = getters

	for _, spec := range specs {
		handleSpecRecursive(module, astFiles, spec, "", "", "", nil, nil, make(map[string]bool))
	}

	return module, nil
}

func formatBuildTags(buildTags string) []string {
	splittedBuildTags := strings.Split(buildTags, ",")
	var formattedBuildTags []string
	for _, tag := range splittedBuildTags {
		if tag != "" {
			formattedBuildTags = append(formattedBuildTags, fmt.Sprintf("go:build %s", tag))
		}
	}
	return formattedBuildTags
}

func newField(allFields map[string]*common.StructField, fieldName string, inputField *common.StructField) string {
	var fieldPath, result string
	for _, node := range strings.Split(inputField.Name, ".") {
		if fieldPath != "" {
			fieldPath += "." + node
		} else {
			fieldPath = node
		}

		if field, ok := allFields[fieldPath]; ok {
			if field.IsOrigTypePtr {
				// process & exec context are set in the template
				if !strings.HasPrefix(fieldName, "process.") && !strings.HasPrefix(fieldName, "exec.") {
					result += fmt.Sprintf("if ev.%s == nil { ev.%s = &%s{} }\n", field.Name, field.Name, field.OrigType)
				}
			} else if field.IsArray && fieldPath != inputField.Name {
				result += fmt.Sprintf("if len(ev.%s) == 0 { ev.%s = append(ev.%s, %s{}) }\n", field.Name, field.Name, field.Name, field.OrigType)
			}
		}
	}

	return result
}

func buildFirstAccessor(allFields map[string]*common.StructField, inputField *common.StructField) string {
	var fieldPath string
	for _, node := range strings.Split(inputField.Name, ".") {
		if fieldPath != "" {
			fieldPath += "." + node
		} else {
			fieldPath = node
		}

		if field, ok := allFields[fieldPath]; ok {
			if field.IsArray && fieldPath != inputField.Name {
				fieldPath += "[0]"
			}
		}
	}

	return "ev." + fieldPath
}

func generatePrefixNilChecks(allFields map[string]*common.StructField, returnType string, field *common.StructField) string {
	var fieldPath, result string
	for _, node := range strings.Split(field.Name, ".") {
		if fieldPath != "" {
			fieldPath += "." + node
		} else {
			fieldPath = node
		}

		if field, ok := allFields[fieldPath]; ok {
			if field.IsOrigTypePtr {
				result += fmt.Sprintf("if ev.%s == nil { return %s }\n", field.Name, getDefaultValueOfType(returnType))
			}
		}
	}

	return result
}

func split(r rune) bool {
	return r == '.' || r == '_'
}

func pascalCaseFieldName(fieldName string) string {
	chunks := strings.FieldsFunc(fieldName, split)
	caser := cases.Title(language.English, cases.NoLower)

	for idx, chunk := range chunks {
		newChunk := chunk
		chunks[idx] = caser.String(newChunk)
	}

	return strings.Join(chunks, "")
}

func getDefaultValueOfType(returnType string) string {
	baseType, isArray := strings.CutPrefix(returnType, "[]")

	if baseType == "int" {
		if isArray {
			return "[]int{}"
		}
		return "0"
	} else if baseType == "int64" {
		if isArray {
			return "[]int64{}"
		}
		return "int64(0)"
	} else if baseType == "uint16" {
		if isArray {
			return "[]uint16{}"
		}
		return "uint16(0)"
	} else if baseType == "uint32" {
		if isArray {
			return "[]uint32{}"
		}
		return "uint32(0)"
	} else if baseType == "uint64" {
		if isArray {
			return "[]uint64{}"
		}
		return "uint64(0)"
	} else if baseType == "bool" {
		if isArray {
			return "[]bool{}"
		}
		return "false"
	} else if baseType == "net.IPNet" {
		if isArray {
			return "[]net.IPNet{}"
		}
		return "net.IPNet{}"
	} else if baseType == "time.Time" {
		if isArray {
			return "[]time.Time{}"
		}
		return "time.Time{}"
	} else if isArray {
		return "[]string{}"
	}
	return `""`
}

func needScrubbed(fieldName string) bool {
	loweredFieldName := strings.ToLower(fieldName)
	if (strings.Contains(loweredFieldName, "argv") && !strings.Contains(loweredFieldName, "argv0")) && !strings.Contains(loweredFieldName, "module") {
		return true
	}
	return false
}

func addSuffixToFuncPrototype(suffix string, prototype string) string {
	chunks := strings.SplitN(prototype, "(", 3)
	chunks = append(chunks[:1], append([]string{suffix, "("}, chunks[1:]...)...)

	return strings.Join(chunks, "")
}

func getFieldHandler(allFields map[string]*common.StructField, field *common.StructField) string {
	if field.Handler == "" || field.Iterator != nil || field.Helper {
		return ""
	}

	if field.Prefix == "" {
		return fmt.Sprintf("ev.FieldHandlers.%s(ev)", field.Handler)
	}

	ptr := "&"
	if allFields[field.Prefix].IsOrigTypePtr {
		ptr = ""
	}

	if field.Ref == "" {
		return fmt.Sprintf("ev.FieldHandlers.%s(ev, %sev.%s)", field.Handler, ptr, field.Prefix)
	}
	return fmt.Sprintf("ev.FieldHandlers.%s(ev, %sev.%s.%s)", field.Handler, ptr, field.Prefix, field.Ref)
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

func getFieldHandlersChecks(allFields map[string]*common.StructField, field *common.StructField) string {
	checks := getChecks(allFields, field)

	var res []string

	if field.SkipADResolution {
		res = append(res, "!forADs")
	}

	if len(field.RestrictedTo) != 0 {
		parts := make([]string, 0)
		for _, restriction := range field.RestrictedTo {
			parts = append(parts, fmt.Sprintf("eventType == \"%s\"", restriction))
		}
		check := strings.Join(parts, " || ")
		if len(parts) > 1 {
			check = fmt.Sprintf("(%s)", check)
		}

		res = append(res, check)
	}

	for _, check := range checks {
		check := fmt.Sprintf("ev.%s()", check)
		res = append(res, check)
	}

	return strings.Join(res, " && ")
}

func getSetHandler(allFields map[string]*common.StructField, field *common.StructField) string {
	var handler string

	fqn := field.Alias

	name := field.Name
	for name != "" {
		field := allFields[name]
		if field == nil {
			break
		}

		if field.SetHandler != "" {
			ptr := ""
			if !field.IsOrigTypePtr {
				ptr = "&"
			}
			return fmt.Sprintf(`%s(%sev.%s, "%s", value)`, field.SetHandler, ptr, field.Name, strings.ReplaceAll(fqn, field.Alias+".", ""))
		}

		idx := strings.LastIndex(name, ".")
		if idx == -1 {
			break
		}
		name = name[:idx]
	}

	return handler
}

func getHandlers(allFields map[string]*common.StructField) map[string]string {
	handlers := make(map[string]string)

	for _, field := range allFields {
		if field.Handler != "" && !field.IsLength {
			returnType := field.ReturnType
			if field.IsArray {
				returnType = "[]" + returnType
			}

			var handler string
			if field.Prefix == "" {
				handler = fmt.Sprintf("%s(ev *Event) %s", field.Handler, returnType)
			} else {
				if field.Ref != "" {
					continue
				}
				handler = fmt.Sprintf("%s(ev *Event, e *%s) %s", field.Handler, field.Struct, returnType)
			}

			if _, exists := handlers[handler]; exists {
				continue
			}

			var name string
			if field.Prefix == "" {
				name = "ev." + field.Name
			} else {
				name = "e" + strings.TrimPrefix(field.Name, field.Prefix)
			}

			if field.ReturnType == "int" {
				if field.IsArray {
					handlers[handler] = fmt.Sprintf("{ var result []int; for _, value := range %s { result = append(result, int(value)) }; return result }", name)
				} else {
					handlers[handler] = fmt.Sprintf("{ return int(%s) }", name)
				}
			} else {
				handlers[handler] = fmt.Sprintf("{ return %s(%s) }", returnType, name)
			}
		}
	}

	return handlers
}

func getFieldRestrictions(field *common.StructField) string {
	if len(field.RestrictedTo) == 0 {
		return "nil"
	}
	return fmt.Sprintf(`[]eval.EventType{"%s"}`, strings.Join(field.RestrictedTo, `", "`))
}

func getFieldReflectType(field *common.StructField) string {
	switch field.ReturnType {
	case "string":
		return "reflect.String"
	case "int":
		return "reflect.Int"
	case "bool":
		return "reflect.Bool"
	case "net.IPNet":
		return "reflect.Struct"
	}
	return ""
}

func isReadOnly(field *common.StructField) bool {
	return field.IsLength || field.ReadOnly
}

func genGetter(getters []string, getter string) bool {
	return slices.Contains(getters, "*") || slices.Contains(getters, getter)
}

func upperCase(str string) string {
	return cases.Title(language.Und).String(str)
}

var funcMap = map[string]interface{}{
	"TrimPrefix":               strings.TrimPrefix,
	"TrimSuffix":               strings.TrimSuffix,
	"HasPrefix":                strings.HasPrefix,
	"NewField":                 newField,
	"BuildFirstAccessor":       buildFirstAccessor,
	"GeneratePrefixNilChecks":  generatePrefixNilChecks,
	"GetFieldHandler":          getFieldHandler,
	"GetChecks":                getChecks,
	"GetFieldHandlersChecks":   getFieldHandlersChecks,
	"GetHandlers":              getHandlers,
	"PascalCaseFieldName":      pascalCaseFieldName,
	"GetDefaultValueOfType":    getDefaultValueOfType,
	"NeedScrubbed":             needScrubbed,
	"AddSuffixToFuncPrototype": addSuffixToFuncPrototype,
	"GetFieldRestrictions":     getFieldRestrictions,
	"GetFieldReflectType":      getFieldReflectType,
	"GetSetHandler":            getSetHandler,
	"IsReadOnly":               isReadOnly,
	"GenGetter":                genGetter,
	"UpperCase":                upperCase,
}

//go:embed accessors.tmpl
var accessorsTemplateCode string

//go:embed field_handlers.tmpl
var fieldHandlersTemplate string

//go:embed field_accessors.tmpl
var perFieldAccessorsTemplate string

func main() {
	module, err := parseFile(modelFile, typesFile, pkgname)
	if err != nil {
		panic(err)
	}

	if len(fieldHandlersOutput) > 0 {
		if err = GenerateContent(fieldHandlersOutput, module, fieldHandlersTemplate); err != nil {
			panic(err)
		}
	}

	if docOutput != "" {
		os.Remove(docOutput)
		if err := doc.GenerateDocJSON(module, path.Dir(modelFile), docOutput); err != nil {
			panic(err)
		}
	}

	os.Remove(output)
	if err := GenerateContent(output, module, accessorsTemplateCode); err != nil {
		panic(err)
	}

	if err := GenerateContent(fieldAccessorsOutput, module, perFieldAccessorsTemplate); err != nil {
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
	inGoCode := false

	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(trimmed, "package") {
			inGoCode = true
		}

		if len(trimmed) != 0 || !inGoCode {
			builder.WriteString(trimmed)
			builder.WriteRune('\n')
		}
	}

	return builder.String()
}

func init() {
	flag.BoolVar(&verbose, "verbose", false, "Be verbose")
	flag.StringVar(&docOutput, "doc", "", "Generate documentation JSON")
	flag.StringVar(&fieldHandlersOutput, "field-handlers", "field_handlers_unix.go", "Field handlers output file")
	flag.StringVar(&modelFile, "input", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&typesFile, "types-file", os.Getenv("TYPESFILE"), "Go type file to use with the model file")
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "unix", "build tags used for parsing")
	flag.StringVar(&fieldAccessorsOutput, "field-accessors-output", "field_accessors_unix.go", "Generated per-field accessors output file")
	flag.StringVar(&output, "output", "accessors_unix.go", "Go generated file")
	flag.Parse()
}
