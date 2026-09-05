// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main generates Workload Protection Agent configuration documentation.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
)

const runtimeSecurityConfigStruct = "RuntimeSecurityConfig"

type setting struct {
	Name         string `json:"name"`
	ConfigKey    string `json:"config_key"`
	EnvVar       string `json:"env_var"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	DefaultValue string `json:"default_value"`
	Visibility   string `json:"visibility"`
}

type documentation struct {
	PublicSettings  []setting `json:"public_settings"`
	WarningSettings []setting `json:"warning_settings"`
}

func main() {
	var (
		input  string
		output string
	)

	flag.StringVar(&input, "input", "", "Path to the runtime security config source file")
	flag.StringVar(&output, "output", "", "Generated JSON documentation file")
	flag.Parse()

	if input == "" || output == "" {
		flag.Usage()
		os.Exit(1)
	}

	doc, err := generateDocumentation(input)
	if err != nil {
		panic(err)
	}

	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic(err)
	}

	if err := os.WriteFile(output, content, 0644); err != nil {
		panic(err)
	}
}

func generateDocumentation(input string) (*documentation, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, input, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("couldn't parse config file: %w", err)
	}

	fieldDocs, fieldTypes, err := parseStructFields(file)
	if err != nil {
		return nil, err
	}

	configKeys, err := parseConfigKeys(file)
	if err != nil {
		return nil, err
	}

	settings := make([]setting, 0)
	for fieldName, meta := range fieldDocs {
		visibility := meta["visibility"]
		if visibility != "public" && visibility != "warning" {
			continue
		}

		setting, err := buildDocumentedSetting(fieldName, meta, fieldTypes, configKeys, visibility)
		if err != nil {
			return nil, err
		}

		settings = append(settings, setting)
	}

	publicSettings := make([]setting, 0)
	warningSettings := make([]setting, 0)
	for _, documentedSetting := range settings {
		switch documentedSetting.Visibility {
		case "public":
			publicSettings = append(publicSettings, documentedSetting)
		case "warning":
			warningSettings = append(warningSettings, documentedSetting)
		}
	}

	sortSettings(publicSettings)
	sortSettings(warningSettings)

	return &documentation{
		PublicSettings:  publicSettings,
		WarningSettings: warningSettings,
	}, nil
}

func buildDocumentedSetting(
	fieldName string,
	meta map[string]string,
	fieldTypes map[string]string,
	configKeys map[string]string,
	visibility string,
) (setting, error) {
	description := meta["description"]
	if description == "" {
		description = meta["description"]
	}
	if description == "" {
		return setting{}, fmt.Errorf("%s setting %q is missing a description", visibility, fieldName)
	}

	configKey := meta["config_key"]
	if configKey == "" {
		var ok bool
		configKey, ok = configKeys[fieldName]
		if !ok {
			return setting{}, fmt.Errorf("couldn't find config key for %s setting %q", visibility, fieldName)
		}
	}

	settingType, ok := fieldTypes[fieldName]
	if !ok || settingType == "" {
		return setting{}, fmt.Errorf("couldn't infer Go type for %s setting %q", visibility, fieldName)
	}

	return setting{
		Name:         fieldName,
		ConfigKey:    configKey,
		EnvVar:       inferEnvVar(configKey),
		Description:  description,
		Type:         settingType,
		DefaultValue: meta["default_value"],
		Visibility:   visibility,
	}, nil
}

// inferEnvVar mirrors pkg/config/nodetreemodel bindEnv/mergeWithEnvPrefix with the
// default agent prefix ("DD") and env key replacer ("." -> "_").
func inferEnvVar(configKey string) string {
	envKey := strings.ToUpper(configKey)
	envKey = strings.ReplaceAll(envKey, ".", "_")
	return "DD_" + envKey
}

func sortSettings(settings []setting) {
	sort.Slice(settings, func(i, j int) bool {
		return settings[i].ConfigKey < settings[j].ConfigKey
	})
}

func parseStructFields(file *ast.File) (map[string]map[string]string, map[string]string, error) {
	docs := make(map[string]map[string]string)
	types := make(map[string]string)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != runtimeSecurityConfigStruct {
				continue
			}

			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				return nil, nil, fmt.Errorf("%s is not a struct", runtimeSecurityConfigStruct)
			}

			for _, field := range structType.Fields.List {
				if len(field.Names) != 1 {
					continue
				}

				meta := parseCommentMetadata(field.Doc)
				if len(meta) == 0 {
					continue
				}

				fieldName := field.Names[0].Name
				docs[fieldName] = meta
				types[fieldName] = formatFieldType(field.Type)
			}
		}
	}

	if len(docs) == 0 {
		return nil, nil, fmt.Errorf("couldn't find documented fields in %s", runtimeSecurityConfigStruct)
	}

	return docs, types, nil
}

func formatFieldType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + formatFieldType(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + formatFieldType(t.Elt)
		}
		return fmt.Sprintf("[%s]%s", formatFieldType(t.Len), formatFieldType(t.Elt))
	case *ast.MapType:
		return "map[" + formatFieldType(t.Key) + "]" + formatFieldType(t.Value)
	case *ast.SelectorExpr:
		return formatFieldType(t.X) + "." + t.Sel.Name
	case *ast.FuncType:
		return "func"
	default:
		return "unknown"
	}
}

func parseCommentMetadata(commentGroup *ast.CommentGroup) map[string]string {
	output := make(map[string]string)
	if commentGroup == nil {
		return output
	}

	for _, comment := range commentGroup.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		key, value, found := strings.Cut(text, ":")
		if !found {
			continue
		}

		output[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}

	return output
}

func parseConfigKeys(file *ast.File) (map[string]string, error) {
	output := make(map[string]string)
	funcConfigKeys := parseFunctionConfigKeys(file)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "NewRuntimeSecurityConfig" || fn.Body == nil {
			continue
		}

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			kv, ok := node.(*ast.KeyValueExpr)
			if !ok {
				return true
			}

			keyIdent, ok := kv.Key.(*ast.Ident)
			if !ok {
				return true
			}

			if configKey := resolveConfigKey(kv.Value, funcConfigKeys); configKey != "" {
				output[keyIdent.Name] = configKey
			}

			return true
		})

		ast.Inspect(fn.Body, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}

			selector, ok := assign.Lhs[0].(*ast.SelectorExpr)
			if !ok {
				return true
			}

			ident, ok := selector.X.(*ast.Ident)
			if !ok || ident.Name != "rsConfig" {
				return true
			}

			if configKey := resolveConfigKey(assign.Rhs[0], funcConfigKeys); configKey != "" {
				output[selector.Sel.Name] = configKey
			}

			return true
		})
	}

	if len(output) == 0 {
		return nil, errors.New("couldn't find config keys in NewRuntimeSecurityConfig")
	}

	return output, nil
}

func resolveConfigKey(expr ast.Expr, funcConfigKeys map[string]string) string {
	if configKey := findConfigKeyInNode(expr); configKey != "" {
		return configKey
	}

	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}

	return funcConfigKeys[ident.Name]
}

func parseFunctionConfigKeys(file *ast.File) map[string]string {
	output := make(map[string]string)

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		if configKey := findConstConfigKey(fn); configKey != "" {
			output[fn.Name.Name] = configKey
			continue
		}

		if configKey := findConfigKeyInNode(fn.Body); configKey != "" {
			output[fn.Name.Name] = configKey
		}
	}

	return output
}

func findConstConfigKey(fn *ast.FuncDecl) string {
	var configKey string

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if configKey != "" {
			return false
		}

		genDecl, ok := node.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			return true
		}

		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Values) != 1 {
				continue
			}

			literal, ok := valueSpec.Values[0].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				continue
			}

			configKey = strings.Trim(literal.Value, `"`)
			return false
		}

		return true
	})

	return configKey
}

func findConfigKeyInNode(node ast.Node) string {
	var configKey string

	ast.Inspect(node, func(n ast.Node) bool {
		if configKey != "" {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !strings.HasPrefix(selector.Sel.Name, "Get") {
			return true
		}

		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}

		configKey = strings.Trim(literal.Value, `"`)
		return false
	})

	return configKey
}
