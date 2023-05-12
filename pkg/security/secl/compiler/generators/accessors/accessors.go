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
	filename               string
	pkgname                string
	output                 string
	verbose                bool
	docOutput              string
	fieldHandlersOutput    string
	buildTags              string
	fieldHandlersBuildTags string
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

func isBasicType(kind string) bool {
	switch kind {
	case "string", "bool", "int", "int8", "int16", "int32", "int64", "uint8", "uint16", "uint32", "uint64", "net.IPNet":
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
func handleBasic(module *common.Module, field seclField, name, alias, aliasPrefix, prefix, kind, event, opOverrides, commentText, containerStructName string, iterator *common.StructField, isArray bool) {
	if verbose {
		fmt.Printf("handleBasic name: %s, kind: %s, alias: %s, isArray: %v\n", name, kind, alias, isArray)
	}

	if prefix != "" {
		name = prefix + "." + name
		alias = aliasPrefix + "." + alias
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
		Struct:      containerStructName,
		Alias:       alias,
		AliasPrefix: aliasPrefix,
	}

	if _, ok := module.EventTypes[event]; !ok {
		module.EventTypes[event] = common.NewEventTypeMetada()
	}

	if field.lengthField {
		name = name + ".length"
		aliasPrefix = alias
		alias = alias + ".length"

		module.Fields[alias] = &common.StructField{
			Name:        name,
			BasicType:   "int",
			ReturnType:  "int",
			OrigType:    "int",
			IsArray:     isArray,
			IsLength:    true,
			Event:       event,
			Iterator:    iterator,
			CommentText: doc.SECLDocForLength,
			OpOverrides: opOverrides,
			Struct:      "string",
			Alias:       alias,
			AliasPrefix: aliasPrefix,
		}
	}
}

// handleEmbedded adds embedded fields to list of exposed SECL fields of the module
func handleEmbedded(module *common.Module, name, prefix, event string, fieldTypeExpr ast.Expr) {
	if verbose {
		log.Printf("handleEmbedded name: %s", name)
	}

	prefixedFieldName := fmt.Sprintf("%s.%s", prefix, name)
	if len(prefix) == 0 {
		prefixedFieldName = name
	}
	fieldType, isPointer, isArray := getFieldIdentName(fieldTypeExpr)

	// maintain a list of all the fields
	module.AllFields[prefixedFieldName] = &common.StructField{
		Name:          prefixedFieldName,
		Event:         event,
		OrigType:      qualifiedType(module, fieldType),
		IsOrigTypePtr: isPointer,
		IsArray:       isArray,
	}
}

// handleNonEmbedded adds non-embedded fields to list of exposed SECL fields of the module
func handleNonEmbedded(module *common.Module, field seclField, prefixedFieldName, event, fieldType string, isPointer, isArray bool) {
	module.AllFields[prefixedFieldName] = &common.StructField{
		Name:          prefixedFieldName,
		Event:         event,
		OrigType:      qualifiedType(module, fieldType),
		IsOrigTypePtr: isPointer,
		IsArray:       isArray,
		Check:         field.check,
	}
}

// handleIterator adds iterator to list of exposed SECL iterators of the module
func handleIterator(module *common.Module, field seclField, fieldType, iterator, aliasPrefix, prefixedFieldName, event, fieldCommentText, opOverrides string, isPointer, isArray bool) *common.StructField {
	alias := field.name
	if aliasPrefix != "" {
		alias = aliasPrefix + "." + field.name
	}

	module.Iterators[alias] = &common.StructField{
		Name:                prefixedFieldName,
		ReturnType:          qualifiedType(module, iterator),
		Event:               event,
		OrigType:            qualifiedType(module, fieldType),
		IsOrigTypePtr:       isPointer,
		IsArray:             isArray,
		Weight:              field.weight,
		CommentText:         fieldCommentText,
		OpOverrides:         opOverrides,
		CachelessResolution: field.cachelessResolution,
		SkipADResolution:    field.skipADResolution,
		Check:               field.check,
	}

	return module.Iterators[alias]
}

// handleFieldWithHandler adds non-embedded fields with handlers to list of exposed SECL fields and event types of the module
func handleFieldWithHandler(module *common.Module, field seclField, aliasPrefix, prefix, prefixedFieldName, fieldType, containerStructName, event, fieldCommentText, opOverrides, handler string, isPointer, isArray bool, fieldIterator *common.StructField) {
	alias := field.name
	if aliasPrefix != "" {
		alias = aliasPrefix + "." + alias
	}

	module.Fields[alias] = &common.StructField{
		Prefix:              prefix,
		Name:                prefixedFieldName,
		BasicType:           origTypeToBasicType(fieldType),
		Struct:              containerStructName,
		Handler:             handler,
		ReturnType:          origTypeToBasicType(fieldType),
		Event:               event,
		OrigType:            fieldType,
		Iterator:            fieldIterator,
		IsArray:             isArray,
		Weight:              field.weight,
		CommentText:         fieldCommentText,
		OpOverrides:         opOverrides,
		CachelessResolution: field.cachelessResolution,
		SkipADResolution:    field.skipADResolution,
		IsOrigTypePtr:       isPointer,
		Check:               field.check,
		Alias:               alias,
		AliasPrefix:         aliasPrefix,
	}

	if field.lengthField {
		var lengthField common.StructField = *module.Fields[alias]
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
	cachelessResolution    bool
	skipADResolution       bool
	lengthField            bool
	weight                 int64
	check                  string
	exposedAtEventRootOnly bool // fields that should only be exposed at the root of an event, i.e. `parent` should not be exposed for an `ancestor` of a process
	containerStructName    string
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

// handleSpecRecursive is a recursive function that walks through the fields of a module
func handleSpecRecursive(module *common.Module, astFile *ast.File, spec interface{}, prefix, aliasPrefix, event string, iterator *common.StructField, dejavu map[string]bool) {
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

	for _, field := range structType.Fields.List {
		fieldCommentText := field.Comment.Text()
		fieldIterator := iterator

		var tag reflect.StructTag
		if field.Tag != nil {
			tag = reflect.StructTag(field.Tag.Value[1 : len(field.Tag.Value)-1])
		}

		if p, ok := tag.Lookup("platform"); ok {
			platform := common.Platform(p)
			compatiblePlatform := platform == common.Unspecified || platform == module.Platform
			if !compatiblePlatform {
				continue
			}
		}

		if e, ok := tag.Lookup("event"); ok {
			event = e
			if _, ok = module.EventTypes[e]; !ok {
				module.EventTypes[e] = common.NewEventTypeMetada()
				dejavu = make(map[string]bool) // clear dejavu map when it's a new event type
			}
			module.EventTypes[e].Doc = fieldCommentText
		}

		if isEmbedded := len(field.Names) == 0; isEmbedded {
			if fieldTag, found := tag.Lookup("field"); found && fieldTag == "-" {
				continue
			}

			ident, _ := field.Type.(*ast.Ident)
			if starExpr, ok := field.Type.(*ast.StarExpr); ident == nil && ok {
				ident, _ = starExpr.X.(*ast.Ident)
			}

			if ident != nil {
				embedded := astFile.Scope.Lookup(ident.Name)
				if embedded != nil {
					handleEmbedded(module, ident.Name, prefix, event, field.Type)

					handleSpecRecursive(module, astFile, embedded.Decl, prefix+"."+ident.Name, aliasPrefix, event, fieldIterator, dejavu)
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
			if tags, err := structtag.Parse(string(tag)); err == nil && len(tags.Tags()) != 0 {
				opOverrides, fields = parseTags(tags, typeSpec.Name.Name)

				if opOverrides == "" && fields == nil {
					continue
				}

			} else {
				fields = append(fields, seclField{name: fieldBasename})
			}

			fieldType, isPointer, isArray := getFieldIdentName(field.Type)

			prefixedFieldName := fmt.Sprintf("%s.%s", prefix, fieldBasename)
			if len(prefix) == 0 {
				prefixedFieldName = fieldBasename
			}

			for _, seclField := range fields {

				handleNonEmbedded(module, seclField, prefixedFieldName, event, fieldType, isPointer, isArray)

				if seclFieldIterator := seclField.iterator; seclFieldIterator != "" {
					fieldIterator = handleIterator(module, seclField, fieldType, seclFieldIterator, aliasPrefix, prefixedFieldName, event, fieldCommentText, opOverrides, isPointer, isArray)
				}

				if handler := seclField.handler; handler != "" {
					handleFieldWithHandler(module, seclField, aliasPrefix, prefix, prefixedFieldName, fieldType, seclField.containerStructName, event, fieldCommentText, opOverrides, handler, isPointer, isArray, fieldIterator)

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

				alias := seclField.name
				if isBasicType(fieldType) {
					handleBasic(module, seclField, fieldBasename, alias, aliasPrefix, prefix, fieldType, event, opOverrides, fieldCommentText, seclField.containerStructName, fieldIterator, isArray)
				} else {
					if symbol, err := resolveSymbol(pkgname, fieldType); err != nil || symbol == nil {
						log.Printf("failed to resolve symbol for %+v in %s", fieldType, pkgname)
					} else {

						spec := astFile.Scope.Lookup(fieldType)
						var newPrefix, newAliasPrefix string
						if prefix != "" {
							newPrefix = prefix + "." + fieldBasename
							newAliasPrefix = aliasPrefix + "." + alias
						} else {
							newPrefix = fieldBasename
							newAliasPrefix = alias
						}

						handleSpecRecursive(module, astFile, spec.Decl, newPrefix, newAliasPrefix, event, fieldIterator, dejavu)
					}
				}

				if !seclField.exposedAtEventRootOnly {
					delete(dejavu, fieldBasename)
				}
			}
		}
	}
}

func parseTags(tags *structtag.Tags, containerStructName string) (string, []seclField) {
	var opOverrides string
	var fields []seclField

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
					return "", nil
				}
				field.containerStructName = containerStructName

				fields = append(fields, field)
			}

		case "op_override":
			opOverrides = tag.Value()
		}
	}

	return opOverrides, fields
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

	formattedBuildTags := formatBuildTags(buildTags)
	formattedFieldHandlersBuildTags := formatBuildTags(fieldHandlersBuildTags)
	for _, comment := range astFile.Comments {
		commentText := comment.Text()
		if strings.HasPrefix(commentText, "+build ") {
			formattedBuildTags = append(formattedBuildTags, commentText)
			formattedFieldHandlersBuildTags = append(formattedFieldHandlersBuildTags, commentText)
		}
	}

	moduleName := path.Base(path.Dir(output))
	if moduleName == "." {
		moduleName = path.Base(pkgName)
	}

	module := &common.Module{
		Name:                   moduleName,
		SourcePkg:              pkgName,
		TargetPkg:              pkgName,
		BuildTags:              formattedBuildTags,
		FieldHandlersBuildTags: formattedFieldHandlersBuildTags,
		Fields:                 make(map[string]*common.StructField),
		AllFields:              make(map[string]*common.StructField),
		Iterators:              make(map[string]*common.StructField),
		EventTypes:             make(map[string]*common.EventTypeMetadata),
		Platform:               common.Unspecified,
	}

	if strings.Contains(buildTags, "linux") {
		module.Platform = common.Linux
	} else if strings.Contains(buildTags, "windows") {
		module.Platform = common.Windows
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
				handleSpecRecursive(module, astFile, spec, "", "", "", nil, make(map[string]bool))
			}
		}
	}

	return module, nil
}

func formatBuildTags(buildTags string) []string {
	splittedBuildTags := strings.Split(buildTags, ",")
	var formattedBuildTags []string
	for _, tag := range splittedBuildTags {
		if tag != "" {
			formattedBuildTags = append(formattedBuildTags, fmt.Sprintf("+build %s", tag))
		}
	}
	return formattedBuildTags
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
				result += fmt.Sprintf("if ev.%s == nil { ev.%s = &%s{} }\n", field.Name, field.Name, field.OrigType)
			}
		}
	}

	return result
}

func getFieldHandler(allFields map[string]*common.StructField, field *common.StructField) string {
	if field.Handler == "" || field.Iterator != nil || field.CachelessResolution {
		return ""
	}

	if field.Prefix == "" {
		return fmt.Sprintf("ev.FieldHandlers.%s(ev)", field.Handler)
	}

	ptr := "&"
	if allFields[field.Prefix].IsOrigTypePtr {
		ptr = ""
	}

	return fmt.Sprintf("ev.FieldHandlers.%s(ev, %sev.%s)", field.Handler, ptr, field.Prefix)
}

func fieldADPrint(field *common.StructField, handler string) string {
	if field.SkipADResolution {
		return fmt.Sprintf("if !forADs { _ = %s }", handler)
	}
	return fmt.Sprintf("_ = %s", handler)
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
				handlers[handler] = fmt.Sprintf("{ return %s }", name)
			}
		}
	}

	return handlers
}

var funcMap = map[string]interface{}{
	"TrimPrefix":      strings.TrimPrefix,
	"TrimSuffix":      strings.TrimSuffix,
	"HasPrefix":       strings.HasPrefix,
	"NewField":        newField,
	"GetFieldHandler": getFieldHandler,
	"FieldADPrint":    fieldADPrint,
	"GetChecks":       getChecks,
	"GetHandlers":     getHandlers,
}

//go:embed accessors.tmpl
var accessorsTemplateCode string

//go:embed field_handlers.tmpl
var fieldHandlersTemplate string

func main() {
	module, err := parseFile(filename, pkgname)
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
	flag.StringVar(&fieldHandlersOutput, "field-handlers", "", "Field handlers output file")
	flag.StringVar(&filename, "input", os.Getenv("GOFILE"), "Go file to generate decoders from")
	flag.StringVar(&pkgname, "package", pkgPrefix+"/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "", "build tags used for parsing")
	flag.StringVar(&fieldHandlersBuildTags, "field-handlers-tags", "", "build tags used for field handlers")
	flag.StringVar(&output, "output", "", "Go generated file")
	flag.Parse()
}
