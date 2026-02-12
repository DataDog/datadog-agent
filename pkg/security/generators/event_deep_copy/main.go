// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates a deep copy function for the Event struct.
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
	"regexp"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/Masterminds/sprig/v3"
	"golang.org/x/tools/go/packages"

	"github.com/DataDog/datadog-agent/pkg/security/generators/accessors/common"
)

// ============================================================================
// Core Data Structures
// ============================================================================

// StructField represents complete type metadata for a struct field.
// Used during code generation to determine the appropriate deep copy strategy.
type StructField struct {
	Name          string          // Field name (may include dot path for nested fields)
	OrigType      string          // Original type name with package qualification
	IsOrigTypePtr bool            // Whether the type itself is a pointer
	IsArray       bool            // Whether the field is a slice or array
	IsFixedArray  bool            // true for [N]T (value type), false for []T (reference type)
	IsMap         bool            // Whether the field is a map
	IsBasic       bool            // Whether this is a primitive or alias of primitive
	ArrayElement  *TypeDescriptor // For arrays/slices: describes the element type
	MapValue      *TypeDescriptor // For maps: describes the value type
}

// TypeDescriptor recursively describes nested type structures.
// Allows analysis of complex types like []*map[string][]MyStruct.
type TypeDescriptor struct {
	TypeName     string          // Simple type name without package prefix
	IsPtr        bool            // Whether this level is a pointer
	IsArray      bool            // Whether this level is an array/slice
	IsFixedArray bool            // true for [N]T (value type), false for []T (reference type)
	IsMap        bool            // Whether this level is a map
	ArrayElement *TypeDescriptor // For nested arrays: element type descriptor
	MapValue     *TypeDescriptor // For nested maps: value type descriptor
}

// FieldNode represents a node in the hierarchical field tree.
// The tree structure mirrors the nested struct hierarchy of the Event type.
type FieldNode struct {
	Name                string                // Simple field name
	FullPath            string                // Dot-separated path from Event root
	Field               *StructField          // Type metadata (nil for intermediate nodes)
	ElementOfArrayField *FieldNode            // For array fields: describes element type and children
	ElementOfMapField   *FieldNode            // For map fields: describes value type and children
	Children            map[string]*FieldNode // Child fields
}

// AstFiles manages AST files and provides lazy loading of external packages.
// External packages are loaded on-demand when their types are referenced.
type AstFiles struct {
	files              []*ast.File       // Loaded AST files
	cfg                *packages.Config  // Package loading configuration
	loadedPackages     map[string]bool   // Tracks load attempts (success or failure)
	packageShortNames  map[string]string // Maps import alias to full package path
	packagePrefixCache map[string]string // Caches package name to qualifier mapping
}

// ============================================================================
// AST Parsing and Symbol Resolution
// ============================================================================

// LookupSymbol searches for a symbol across all loaded AST files.
// If not found, attempts to lazy-load external packages to find the symbol.
func (af *AstFiles) LookupSymbol(symbol string) *ast.Object { //nolint:staticcheck
	// Search in currently loaded files
	for _, file := range af.files {
		if obj := file.Scope.Lookup(symbol); obj != nil {
			return obj
		}
	}

	// Attempt lazy loading of external packages
	for _, importPath := range af.packageShortNames {
		if af.loadedPackages[importPath] {
			continue
		}

		if err := af.loadExternalPackage(importPath); err != nil {
			continue
		}

		// Retry search in newly loaded files
		for _, file := range af.files {
			if obj := file.Scope.Lookup(symbol); obj != nil {
				return obj
			}
		}
	}

	return nil
}

// loadExternalPackage loads an external package's AST files and appends them to the AstFiles.
func (af *AstFiles) loadExternalPackage(pkgPath string) error {
	if af.loadedPackages[pkgPath] {
		return nil
	}

	af.loadedPackages[pkgPath] = true

	pkgs, err := packages.Load(af.cfg, pkgPath)
	if err != nil {
		return fmt.Errorf("failed to load package %s: %w", pkgPath, err)
	}

	for _, pkg := range pkgs {
		if pkg.Syntax != nil {
			af.files = append(af.files, pkg.Syntax...)
			if verbose {
				log.Printf("Lazy-loaded external package %s (%d files)", pkgPath, len(pkg.Syntax))
			}
		}
	}

	return nil
}

// GetPackageForType determines the package prefix for a given type name.
// Searches loaded AST files and lazy-loads external packages as needed.
// Returns the package prefix (e.g., "eval.", "utils.") or empty string for model package types.
func (af *AstFiles) GetPackageForType(typeName string) string {
	// Search in currently loaded files
	if prefix := af.searchLoadedFiles(typeName); prefix != "" || af.typeFoundInModel(typeName) {
		return prefix
	}

	// Type not found - attempt lazy loading from imports
	return af.lazyLoadAndSearch(typeName)
}

// searchLoadedFiles looks for a type in already loaded AST files and returns its package prefix
func (af *AstFiles) searchLoadedFiles(typeName string) string {
	for _, file := range af.files {
		if obj := file.Scope.Lookup(typeName); obj != nil && file.Name != nil {
			pkgName := file.Name.Name

			if prefix, cached := af.packagePrefixCache[pkgName]; cached {
				return prefix
			}

			prefix := af.getPrefixForPackage(pkgName)
			af.packagePrefixCache[pkgName] = prefix
			return prefix
		}
	}
	return ""
}

// typeFoundInModel checks if the type was found in the model package
func (af *AstFiles) typeFoundInModel(typeName string) bool {
	for _, file := range af.files {
		if obj := file.Scope.Lookup(typeName); obj != nil && file.Name != nil {
			return file.Name.Name == "model"
		}
	}
	return false
}

// lazyLoadAndSearch attempts to lazy-load external packages and search for the type
func (af *AstFiles) lazyLoadAndSearch(typeName string) string {
	for shortName, importPath := range af.packageShortNames {
		if af.loadedPackages[importPath] {
			continue
		}

		if err := af.loadExternalPackage(importPath); err != nil {
			if verbose {
				log.Printf("Warning: %v", err)
			}
			continue
		}

		// Search in newly loaded files
		for _, file := range af.files {
			if obj := file.Scope.Lookup(typeName); obj != nil {
				prefix := shortName + "."
				af.packagePrefixCache[shortName] = prefix
				return prefix
			}
		}
	}
	return ""
}

// getPrefixForPackage returns the appropriate prefix for a package name
func (af *AstFiles) getPrefixForPackage(pkgName string) string {
	if pkgName == "model" {
		return ""
	}
	return pkgName + "."
}

// Parse extracts specs from AST files
func (af *AstFiles) Parse() []ast.Spec {
	var specs []ast.Spec

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
			}

			if !genaccessors {
				continue
			}

			specs = append(specs, decl.Specs...)
		}
	}

	return specs
}

// newAstFiles loads the primary package and discovers its imports for lazy loading.
// External packages are loaded on-demand when their types are encountered during analysis.
func newAstFiles(cfg *packages.Config, primaryFile string) (*AstFiles, error) {
	pkgs, err := packages.Load(cfg, "file="+primaryFile)
	if err != nil {
		return nil, err
	}

	if len(pkgs) == 0 || len(pkgs[0].Syntax) == 0 {
		return nil, fmt.Errorf("failed to get syntax from package containing %s", primaryFile)
	}

	astFiles := &AstFiles{
		files:              pkgs[0].Syntax,
		cfg:                cfg,
		loadedPackages:     make(map[string]bool),
		packageShortNames:  make(map[string]string),
		packagePrefixCache: make(map[string]string),
	}

	astFiles.discoverImports(pkgs[0].Syntax)

	if verbose {
		log.Printf("Loaded primary package with %d files and %d imports",
			len(pkgs[0].Syntax), len(astFiles.packageShortNames))
	}

	return astFiles, nil
}

// discoverImports extracts import declarations from AST files to enable lazy loading
func (af *AstFiles) discoverImports(files []*ast.File) {
	for _, file := range files {
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			shortName := af.extractPackageName(imp, importPath)

			af.packageShortNames[shortName] = importPath

			if verbose {
				log.Printf("Found import: %s → %s", shortName, importPath)
			}
		}
	}
}

// extractPackageName determines the short name for an imported package
func (af *AstFiles) extractPackageName(imp *ast.ImportSpec, importPath string) string {
	if imp.Name != nil {
		return imp.Name.Name // Explicit alias
	}

	// Use last component of path
	parts := strings.Split(importPath, "/")
	return parts[len(parts)-1]
}

// getFieldName extracts the field name from an AST expression
func getFieldName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return getFieldName(expr.X)
	case *ast.ArrayType:
		return getFieldName(expr.Elt)
	case *ast.MapType:
		return getFieldName(expr.Value)
	case *ast.SelectorExpr:
		// Get the full qualified name
		fullName := getFieldName(expr.X) + "." + getFieldName(expr.Sel)
		// Strip package prefix if it's a built-in type (e.g., "net.byte" -> "byte")
		if dotIdx := strings.LastIndex(fullName, "."); dotIdx >= 0 {
			baseName := fullName[dotIdx+1:]
			if isBuiltinType(baseName) {
				return baseName
			}
		}
		return fullName
	default:
		return ""
	}
}

// resolveTypeAlias resolves type aliases to their underlying type expressions.
// Example: eval.MatchingSubExprs → []eval.MatchingSubExpr
func resolveTypeAlias(astFiles *AstFiles, expr ast.Expr) ast.Expr {
	typeName := extractTypeName(expr)
	if typeName == "" {
		return expr
	}

	obj := astFiles.LookupSymbol(typeName)
	if obj == nil || obj.Decl == nil {
		return expr
	}

	typeSpec, ok := obj.Decl.(*ast.TypeSpec)
	if !ok {
		return expr
	}

	return typeSpec.Type
}

// extractTypeName extracts the simple type name from an expression
func extractTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	default:
		return ""
	}
}

// ============================================================================
// Type Analysis and Metadata Extraction
// ============================================================================

// getFieldType analyzes a field's complete type structure.
// Returns nil for interface types, otherwise returns a StructField with type metadata.
func getFieldType(expr ast.Expr, astFiles *AstFiles) *StructField {
	pkgPrefix := extractPackagePrefix(expr)
	resolvedExpr := resolveTypeAlias(astFiles, expr)

	if isInterface(resolvedExpr) {
		return nil
	}

	if mapType, ok := resolvedExpr.(*ast.MapType); ok {
		return analyzeMapField(mapType, pkgPrefix)
	}

	if arrayType, ok := resolvedExpr.(*ast.ArrayType); ok {
		return analyzeArrayField(arrayType, pkgPrefix)
	}

	if starExpr, ok := resolvedExpr.(*ast.StarExpr); ok {
		return analyzePointerField(starExpr, pkgPrefix)
	}

	return &StructField{OrigType: getFieldName(expr)}
}

// extractPackagePrefix extracts the package qualifier from a selector expression
func extractPackagePrefix(expr ast.Expr) string {
	if selectorExpr, ok := expr.(*ast.SelectorExpr); ok {
		return getFieldName(selectorExpr.X) + "."
	}
	return ""
}

// isInterface checks if an expression represents an interface type
func isInterface(expr ast.Expr) bool {
	_, ok := expr.(*ast.InterfaceType)
	return ok
}

// analyzeMapField analyzes a map type and returns its StructField
func analyzeMapField(mapType *ast.MapType, pkgPrefix string) *StructField {
	descriptor := analyzeTypeDescriptor(mapType.Value)
	if descriptor == nil {
		return nil // Map value is an interface
	}

	typeName := getFieldName(mapType.Value)
	// Don't add package prefix to built-in types
	// Also strip any package prefix that's already in the type name for built-in types
	if strings.Contains(typeName, ".") {
		baseName := typeName[strings.LastIndex(typeName, ".")+1:]
		if isBuiltinType(baseName) {
			typeName = baseName
		}
	}
	if isBuiltinType(typeName) {
		pkgPrefix = ""
	}

	return &StructField{
		IsMap:    true,
		MapValue: descriptor,
		OrigType: pkgPrefix + typeName,
	}
}

// analyzeArrayField analyzes an array/slice type and returns its StructField
func analyzeArrayField(arrayType *ast.ArrayType, pkgPrefix string) *StructField {
	descriptor := analyzeTypeDescriptor(arrayType.Elt)
	if descriptor == nil {
		return nil // Array element is an interface
	}

	typeName := getFieldName(arrayType.Elt)
	// Don't add package prefix to built-in types
	// Also strip any package prefix that's already in the type name for built-in types
	if strings.Contains(typeName, ".") {
		baseName := typeName[strings.LastIndex(typeName, ".")+1:]
		if isBuiltinType(baseName) {
			typeName = baseName
		}
	}
	if isBuiltinType(typeName) {
		pkgPrefix = ""
	}

	return &StructField{
		IsArray:      true,
		IsFixedArray: arrayType.Len != nil,
		ArrayElement: descriptor,
		OrigType:     pkgPrefix + typeName,
	}
}

// analyzePointerField analyzes a pointer type and returns its StructField
func analyzePointerField(starExpr *ast.StarExpr, pkgPrefix string) *StructField {
	typeName := getFieldName(starExpr.X)
	// Don't add package prefix to built-in types
	// Also strip any package prefix that's already in the type name for built-in types
	if strings.Contains(typeName, ".") {
		baseName := typeName[strings.LastIndex(typeName, ".")+1:]
		if isBuiltinType(baseName) {
			typeName = baseName
		}
	}
	if isBuiltinType(typeName) {
		pkgPrefix = ""
	}

	return &StructField{
		IsOrigTypePtr: true,
		OrigType:      pkgPrefix + typeName,
	}
}

// analyzeTypeDescriptor recursively analyzes a type expression to extract its structure.
// Returns nil if the type contains interfaces at any level.
func analyzeTypeDescriptor(expr ast.Expr) *TypeDescriptor {
	desc := &TypeDescriptor{}

	// Handle pointer wrapper
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		desc.IsPtr = true
		expr = starExpr.X
	}

	// Skip interfaces
	if isInterface(expr) {
		return nil
	}

	// Analyze composite types
	if arrayType, ok := expr.(*ast.ArrayType); ok {
		return analyzeArrayDescriptor(arrayType, desc)
	}

	if mapType, ok := expr.(*ast.MapType); ok {
		return analyzeMapDescriptor(mapType, desc)
	}

	// Simple type
	desc.TypeName = getFieldName(expr)
	// Strip package prefix from built-in types in TypeName
	if strings.Contains(desc.TypeName, ".") {
		baseName := desc.TypeName[strings.LastIndex(desc.TypeName, ".")+1:]
		if isBuiltinType(baseName) {
			desc.TypeName = baseName
		}
	}
	return desc
}

// analyzeArrayDescriptor analyzes array/slice element types recursively
func analyzeArrayDescriptor(arrayType *ast.ArrayType, desc *TypeDescriptor) *TypeDescriptor {
	if isInterface(arrayType.Elt) {
		return nil // Skip arrays of interfaces
	}

	elementDesc := analyzeTypeDescriptor(arrayType.Elt)
	if elementDesc == nil {
		return nil // Element contains interfaces
	}

	desc.IsArray = true
	desc.IsFixedArray = (arrayType.Len != nil)
	desc.TypeName = getFieldName(arrayType.Elt)
	desc.ArrayElement = elementDesc
	return desc
}

// analyzeMapDescriptor analyzes map value types recursively
func analyzeMapDescriptor(mapType *ast.MapType, desc *TypeDescriptor) *TypeDescriptor {
	if isInterface(mapType.Value) {
		return nil // Skip maps with interface values
	}

	valueDesc := analyzeTypeDescriptor(mapType.Value)
	if valueDesc == nil {
		return nil // Value contains interfaces
	}

	desc.IsMap = true
	desc.TypeName = getFieldName(mapType.Value)
	desc.MapValue = valueDesc
	return desc
}

// qualifiedType adds package qualification to a type name if needed.
// Primitive types remain unqualified.
func qualifiedType(module *common.Module, kind string) string {
	// Built-in types should never be package-qualified
	if isBuiltinType(kind) {
		return kind
	}

	// Check if this is a qualified built-in type (e.g., "net.byte") and strip the package
	if strings.Contains(kind, ".") {
		baseName := kind[strings.LastIndex(kind, ".")+1:]
		if isBuiltinType(baseName) {
			return baseName
		}
		// Already qualified non-built-in types (e.g., "net.IPNet", "time.Duration") - return as-is
		return kind
	}

	return module.SourcePkgPrefix + kind
}

// isBasicType checks if a type string represents a basic type or commonly used stdlib alias.
// Includes primitive types, stdlib types (time, net), and external package types (containerutils).
func isBasicType(kind string) bool {
	switch kind {
	case "string", "bool", "int", "int8", "int16", "int32", "int64",
		"uint8", "uint16", "uint32", "uint64", "byte", "net.IPNet":
		return true
	case "time.Duration", "time.Time":
		return true
	case "containerutils.CGroupID", "containerutils.ContainerID":
		return true
	}
	return false
}

// isBuiltinType checks if a type name is a Go built-in type that should never be package-qualified
func isBuiltinType(typeName string) bool {
	switch typeName {
	case "string", "bool", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"byte", "rune", "float32", "float64", "complex64", "complex128":
		return true
	}
	return false
}

// isInterfaceType checks if a type name represents an interface type
func isInterfaceType(astFiles *AstFiles, typeName string) bool {
	if typeName == "error" {
		return true
	}

	spec := astFiles.LookupSymbol(typeName)
	if spec == nil {
		return false
	}

	if typeSpec, ok := spec.Decl.(*ast.TypeSpec); ok {
		_, isIface := typeSpec.Type.(*ast.InterfaceType)
		return isIface
	}

	return false
}

// isBasicOrAliasType checks if a type is a primitive or an alias to a primitive.
// Examples: int, string, type HashState int, type Duration time.Duration
func isBasicOrAliasType(astFiles *AstFiles, typeName string) bool {
	if isBasicType(typeName) {
		return true
	}

	obj := astFiles.LookupSymbol(typeName)
	if obj == nil {
		return false
	}

	typeSpec, ok := obj.Decl.(*ast.TypeSpec)
	if !ok {
		return false
	}

	return isUnderlyingTypeBasic(typeSpec.Type)
}

// isUnderlyingTypeBasic checks if the underlying type is a basic type
func isUnderlyingTypeBasic(typeExpr ast.Expr) bool {
	switch underlyingType := typeExpr.(type) {
	case *ast.Ident:
		return isBasicType(underlyingType.Name)
	case *ast.SelectorExpr:
		if x, ok := underlyingType.X.(*ast.Ident); ok {
			qualifiedName := x.Name + "." + underlyingType.Sel.Name
			return isBasicType(qualifiedName)
		}
	}
	return false
}

// buildAllStructFields recursively populates module.AllStructFields with all exported struct fields.
func buildAllStructFields(module *common.Module, astFiles *AstFiles, rootSpec interface{}, prefix string, visited map[string]bool) {
	typeSpec, ok := rootSpec.(*ast.TypeSpec)
	if !ok {
		return
	}

	structType, ok := typeSpec.Type.(*ast.StructType)
	if !ok {
		return
	}

	allStructFields := module.AllStructFields.(map[string]*StructField)

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 {
			processEmbeddedField(field, module, astFiles, prefix, visited, allStructFields)
		} else {
			processRegularField(field, module, astFiles, prefix, visited, allStructFields)
		}
	}
}

// processEmbeddedField handles embedded struct fields
func processEmbeddedField(field *ast.Field, module *common.Module, astFiles *AstFiles, prefix string, visited map[string]bool, allStructFields map[string]*StructField) {
	ident := extractEmbeddedIdent(field.Type)
	if ident == nil {
		return
	}

	fieldInfo, fullName, _ := prepareFieldInfo(field, ident.Name, prefix, module, astFiles)
	if fieldInfo == nil || visited[fullName] {
		return
	}

	// Add to collection
	fieldInfo.Name = fullName
	allStructFields[fullName] = fieldInfo

	// Recurse into embedded type
	recurseIntoType(ident.Name, fullName, fullName, module, astFiles, visited)
}

// processRegularField handles named struct fields
func processRegularField(field *ast.Field, module *common.Module, astFiles *AstFiles, prefix string, visited map[string]bool, allStructFields map[string]*StructField) {
	fieldBasename := field.Names[0].Name

	// Skip non-exported fields
	if !unicode.IsUpper(rune(fieldBasename[0])) || visited[fieldBasename] {
		return
	}

	fullName := buildFullFieldName(fieldBasename, prefix)
	fieldInfo, _, unqualifiedType := prepareFieldInfo(field, fieldBasename, prefix, module, astFiles)
	if fieldInfo == nil {
		return
	}

	// Add to collection
	fieldInfo.Name = fullName
	allStructFields[fullName] = fieldInfo

	// Recurse into complex types
	recurseIntoType(unqualifiedType, fullName, fieldBasename, module, astFiles, visited)
}

// extractEmbeddedIdent extracts the identifier from an embedded field type
func extractEmbeddedIdent(fieldType ast.Expr) *ast.Ident {
	if ident, ok := fieldType.(*ast.Ident); ok {
		return ident
	}
	if starExpr, ok := fieldType.(*ast.StarExpr); ok {
		if ident, ok := starExpr.X.(*ast.Ident); ok {
			return ident
		}
	}
	return nil
}

// prepareFieldInfo analyzes a field and prepares its metadata with proper qualification
func prepareFieldInfo(field *ast.Field, basename, prefix string, module *common.Module, astFiles *AstFiles) (*StructField, string, string) {
	fieldInfo := getFieldType(field.Type, astFiles)
	if fieldInfo == nil || shouldSkipInterface(astFiles, fieldInfo.OrigType) {
		return nil, "", ""
	}

	fullName := buildFullFieldName(basename, prefix)
	fieldInfo.IsBasic = isBasicOrAliasType(astFiles, fieldInfo.OrigType)

	unqualifiedType := stripPackageQualifier(fieldInfo.OrigType)
	qualifyFieldType(fieldInfo, unqualifiedType, module, astFiles)

	return fieldInfo, fullName, unqualifiedType
}

// buildFullFieldName constructs the full field name with prefix
func buildFullFieldName(basename, prefix string) string {
	if prefix != "" {
		return prefix + "." + basename
	}
	return basename
}

// stripPackageQualifier removes the package prefix from a type name
func stripPackageQualifier(typeName string) string {
	if dotIdx := strings.LastIndex(typeName, "."); dotIdx >= 0 {
		return typeName[dotIdx+1:]
	}
	return typeName
}

// shouldSkipInterface checks if a type should be skipped (interfaces)
func shouldSkipInterface(astFiles *AstFiles, typeName string) bool {
	return typeName == "FieldHandlers" || isInterfaceType(astFiles, typeName)
}

// qualifyFieldType adds appropriate package qualification to a field's type
func qualifyFieldType(fieldInfo *StructField, unqualifiedType string, module *common.Module, astFiles *AstFiles) {
	if strings.Contains(fieldInfo.OrigType, ".") || fieldInfo.IsBasic {
		fieldInfo.OrigType = qualifiedType(module, fieldInfo.OrigType)
		return
	}

	pkgPrefix := astFiles.GetPackageForType(unqualifiedType)
	if pkgPrefix != "" {
		fieldInfo.OrigType = pkgPrefix + fieldInfo.OrigType
	} else {
		fieldInfo.OrigType = qualifiedType(module, fieldInfo.OrigType)
	}

	// Final cleanup: strip package prefix from built-in types that may have been incorrectly qualified
	// This handles cases like "net.byte" -> "byte"
	if strings.Contains(fieldInfo.OrigType, ".") {
		baseName := fieldInfo.OrigType[strings.LastIndex(fieldInfo.OrigType, ".")+1:]
		if isBuiltinType(baseName) {
			fieldInfo.OrigType = baseName
		}
	}
}

// recurseIntoType recursively processes a field's type if it's a struct
func recurseIntoType(lookupName, recursePath, visitedKey string, module *common.Module, astFiles *AstFiles, visited map[string]bool) {
	visited[visitedKey] = true
	defer delete(visited, visitedKey)

	spec := astFiles.LookupSymbol(lookupName)
	if spec != nil {
		buildAllStructFields(module, astFiles, spec.Decl, recursePath, visited)
	}
}

// ============================================================================
// Hierarchical Field Tree Construction
// ============================================================================

// buildFieldTree constructs a tree representation of the Event struct hierarchy.
// The tree is used by the template to recursively generate deep copy functions.
func buildFieldTree(allFields interface{}, astFiles *AstFiles) *FieldNode {
	fields := allFields.(map[string]*StructField)

	root := &FieldNode{
		Name:     "Event",
		FullPath: "",
		Children: make(map[string]*FieldNode),
	}

	var fieldNames []string
	for name := range fields {
		fieldNames = append(fieldNames, name)
	}
	slices.Sort(fieldNames)

	for _, fieldName := range fieldNames {
		field := fields[fieldName]
		parts := strings.Split(field.Name, ".")

		current := root
		currentPath := ""

		for i, part := range parts {
			if currentPath != "" {
				currentPath += "."
			}
			currentPath += part
			if _, exists := current.Children[part]; !exists {
				current.Children[part] = &FieldNode{
					Name:     part,
					FullPath: currentPath,
					Field:    nil,
					Children: make(map[string]*FieldNode),
				}
			}

			current = current.Children[part]

			if i == len(parts)-1 {
				current.Field = field
				attachArrayElementNode(current, field, currentPath, astFiles)
				attachMapValueNode(current, field, currentPath, astFiles)
			}
		}
	}

	return root
}

// attachArrayElementNode creates and attaches an ElementOfArrayField node for array types
func attachArrayElementNode(parent *FieldNode, field *StructField, currentPath string, astFiles *AstFiles) {
	if !field.IsArray || field.ArrayElement == nil {
		return
	}

	elementOrigType := field.OrigType
	// Strip package prefix from built-in types in element OrigType
	if strings.Contains(elementOrigType, ".") {
		baseName := elementOrigType[strings.LastIndex(elementOrigType, ".")+1:]
		if isBuiltinType(baseName) {
			elementOrigType = baseName
		}
	}

	parent.ElementOfArrayField = &FieldNode{
		Name:     elementOrigType,
		FullPath: currentPath + "[]",
		Field: &StructField{
			Name:          elementOrigType,
			OrigType:      elementOrigType,
			IsArray:       field.ArrayElement.IsArray,
			IsFixedArray:  field.ArrayElement.IsFixedArray,
			IsMap:         field.ArrayElement.IsMap,
			IsOrigTypePtr: field.ArrayElement.IsPtr,
			IsBasic:       isBasicOrAliasType(astFiles, field.ArrayElement.TypeName),
			ArrayElement:  field.ArrayElement.ArrayElement,
			MapValue:      field.ArrayElement.MapValue,
		},
		Children: parent.Children,
	}
}

// attachMapValueNode creates and attaches an ElementOfMapField node for map types
func attachMapValueNode(parent *FieldNode, field *StructField, currentPath string, astFiles *AstFiles) {
	if !field.IsMap || field.MapValue == nil {
		return
	}

	parent.ElementOfMapField = &FieldNode{
		Name:     field.OrigType,
		FullPath: currentPath + "[string]",
		Field: &StructField{
			Name:          field.OrigType,
			OrigType:      field.OrigType,
			IsArray:       field.MapValue.IsArray,
			IsMap:         field.MapValue.IsMap,
			IsOrigTypePtr: field.MapValue.IsPtr,
			IsBasic:       isBasicOrAliasType(astFiles, field.MapValue.TypeName),
			ArrayElement:  field.MapValue.ArrayElement,
			MapValue:      field.MapValue.MapValue,
		},
		Children: parent.Children,
	}
}

// printFieldTreeDebug prints the field tree for debugging
func printFieldTreeDebug(node *FieldNode, prefix string, isLast bool) {
	if node.Name != "Event" {
		connector := "├── "
		if isLast {
			connector = "└── "
		}

		typeInfo := ""
		strategyInfo := ""
		elementInfo := ""
		if node.Field != nil {
			typeInfo = " [" + node.Field.OrigType
			if node.Field.IsOrigTypePtr {
				typeInfo += "*"
			}
			if node.Field.IsArray {
				typeInfo += "[]"
			}
			if node.Field.IsMap {
				typeInfo += "map"
			}
			typeInfo += "]"

			strategy := getFieldCopyStrategy(node.Field)
			strategyInfo = fmt.Sprintf(" <%s>", strategy)

			if node.ElementOfArrayField != nil {
				if node.ElementOfArrayField.Field.IsOrigTypePtr {
					elementInfo = fmt.Sprintf(" {arr-elem: %s*, children: %d}",
						node.ElementOfArrayField.Field.OrigType,
						len(node.ElementOfArrayField.Children))
				} else {
					elementInfo = fmt.Sprintf(" {arr-elem: %s, children: %d}",
						node.ElementOfArrayField.Field.OrigType,
						len(node.ElementOfArrayField.Children))
				}
			}

			if node.ElementOfMapField != nil {
				mapValueInfo := node.ElementOfMapField.Field.OrigType
				if node.ElementOfMapField.Field.IsArray {
					mapValueInfo = "[]" + mapValueInfo
				}
				if node.ElementOfMapField.Field.IsOrigTypePtr {
					mapValueInfo = "*" + mapValueInfo
				}
				if elementInfo != "" {
					elementInfo += " "
				}
				elementInfo += fmt.Sprintf("{map-value: %s, children: %d}",
					mapValueInfo,
					len(node.ElementOfMapField.Children))
			}
		}

		fmt.Printf("%s%s%s%s%s%s\n", prefix, connector, node.Name, typeInfo, strategyInfo, elementInfo)
	} else {
		fmt.Printf("=== %s ===\n", node.Name)
	}

	var childNames []string
	for name := range node.Children {
		childNames = append(childNames, name)
	}
	slices.Sort(childNames)

	for i, childName := range childNames {
		child := node.Children[childName]
		isLastChild := i == len(childNames)-1

		newPrefix := prefix
		if node.Name != "Event" {
			if isLast {
				newPrefix += "    "
			} else {
				newPrefix += "│   "
			}
		}

		printFieldTreeDebug(child, newPrefix, isLastChild)
	}
}

// ============================================================================
// Template Helper Functions
// ============================================================================

// getFieldCopyStrategy determines the copy approach based on field type.
// Returns a strategy name used by the template to select the appropriate copy logic.
func getFieldCopyStrategy(field *StructField) string {
	if field.IsMap {
		return "map"
	}
	if field.IsArray {
		return "array"
	}
	if field.IsOrigTypePtr {
		return "pointer"
	}
	if field.IsBasic {
		return "value"
	}
	return "struct"
}

// ============================================================================
// Code Generation and Output
// ============================================================================

// removeEmptyLines cleans up the generated code by removing blank lines.
// Preserves formatting before the package declaration.
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

// formatBuildTags converts a comma-separated build tag string into go:build directives.
// Example: "unix,linux" → ["go:build unix", "go:build linux"]
func formatBuildTags(buildTags string) []string {
	tags := strings.Split(buildTags, ",")
	var formatted []string
	for _, tag := range tags {
		if tag != "" {
			formatted = append(formatted, "go:build "+tag)
		}
	}
	return formatted
}

// ============================================================================
// Template
// ============================================================================

//go:embed event_deep_copy.tmpl
var eventDeepCopyTemplate string

// ============================================================================
// Configuration and Main
// ============================================================================

var (
	modelFile string
	typesFile string
	pkgname   string
	output    string
	verbose   bool
	buildTags string
)

func init() {
	flag.StringVar(&modelFile, "input", os.Getenv("GOFILE"), "Go file to generate from")
	flag.StringVar(&typesFile, "types-file", os.Getenv("TYPESFILE"), "Go type file to use with the model file")
	flag.StringVar(&pkgname, "package", "github.com/DataDog/datadog-agent/pkg/security/secl/"+os.Getenv("GOPACKAGE"), "Go package name")
	flag.StringVar(&buildTags, "tags", "unix", "build tags used for parsing")
	flag.StringVar(&output, "output", "event_deep_copy_unix.go", "Generated output file")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
	flag.Parse()
}

func main() {
	cfg := &packages.Config{
		Mode:       packages.NeedSyntax | packages.NeedTypes | packages.NeedImports,
		BuildFlags: []string{"-mod=readonly", "-tags=" + buildTags},
	}

	astFiles, err := newAstFiles(cfg, modelFile)
	if err != nil {
		panic(err)
	}

	module := createModule()
	populateStructFields(module, astFiles)

	fieldTree := buildFieldTree(module.AllStructFields, astFiles)
	if verbose {
		printFieldTreeDebug(fieldTree, "", false)
	}

	generateCode(module, fieldTree)
	fmt.Printf("Generated: %s\n", output)
}

// createModule initializes the module configuration
func createModule() *common.Module {
	moduleName := determineModuleName()

	module := &common.Module{
		Name:            moduleName,
		SourcePkg:       pkgname,
		TargetPkg:       pkgname,
		BuildTags:       formatBuildTags(buildTags),
		AllStructFields: make(map[string]*StructField),
	}

	if moduleName != path.Base(pkgname) {
		module.SourcePkgPrefix = path.Base(pkgname) + "."
		module.TargetPkg = path.Clean(path.Join(pkgname, path.Dir(output)))
	}

	return module
}

// determineModuleName derives the module name from the output path
func determineModuleName() string {
	moduleName := path.Base(path.Dir(output))
	if moduleName == "." {
		return path.Base(pkgname)
	}
	return moduleName
}

// populateStructFields analyzes the AST and populates AllStructFields
func populateStructFields(module *common.Module, astFiles *AstFiles) {
	specs := astFiles.Parse()
	for _, spec := range specs {
		buildAllStructFields(module, astFiles, spec, "", make(map[string]bool))
	}
}

// TemplateData combines module metadata with the generated field tree
type TemplateData struct {
	*common.Module
	FieldTree *FieldNode
}

// generateCode executes the template and writes the output file
func generateCode(module *common.Module, fieldTree *FieldNode) {
	templateData := TemplateData{
		Module:    module,
		FieldTree: fieldTree,
	}

	tmpl := createTemplate()
	generatedCode := executeTemplate(tmpl, templateData)
	writeFormattedOutput(generatedCode)
}

// createTemplate initializes the code generation template with custom functions
func createTemplate() *template.Template {
	funcMap := template.FuncMap{
		"GetFieldCopyStrategy": getFieldCopyStrategy,
		"BaseName":             extractBaseName,
	}
	return template.Must(template.New("deep_copy").
		Funcs(funcMap).
		Funcs(sprig.TxtFuncMap()).
		Parse(eventDeepCopyTemplate))
}

// extractBaseName returns the last component of a dotted path
func extractBaseName(s string) string {
	parts := strings.Split(s, ".")
	return parts[len(parts)-1]
}

// executeTemplate runs the template and returns cleaned output
func executeTemplate(tmpl *template.Template, data TemplateData) string {
	buffer := bytes.Buffer{}
	if err := tmpl.Execute(&buffer, data); err != nil {
		panic(err)
	}
	output := removeEmptyLines(&buffer)

	// Final cleanup: fix any package-qualified built-in types that slipped through
	// This handles patterns like "[]net.byte" -> "[]byte", "*pkg.int" -> "*int", etc.
	for _, builtinType := range []string{"byte", "rune", "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64",
		"complex64", "complex128", "bool", "string"} {
		// Replace patterns like "net.byte" with just "byte"
		// Match package.builtin where package is one or more word characters
		pattern := `(\w+)\.` + builtinType + `\b`
		re := regexp.MustCompile(pattern)
		output = re.ReplaceAllString(output, builtinType)
	}

	return output
}

// writeFormattedOutput writes the generated code to a temp file, formats it, and renames it
func writeFormattedOutput(content string) {
	tmpfile, err := os.CreateTemp(path.Dir(output), "event_deep_copy")
	if err != nil {
		panic(err)
	}
	defer os.Remove(tmpfile.Name())

	if _, err := tmpfile.WriteString(content); err != nil {
		panic(err)
	}

	if err := tmpfile.Close(); err != nil {
		panic(err)
	}

	formatWithGofmt(tmpfile.Name())

	if err := os.Rename(tmpfile.Name(), output); err != nil {
		panic(err)
	}
}

// formatWithGofmt runs gofmt on the specified file
func formatWithGofmt(filename string) {
	cmd := exec.Command("gofmt", "-s", "-w", filename)
	if gofmtOutput, err := cmd.CombinedOutput(); err != nil {
		log.Fatal(string(gofmtOutput))
	}
}
