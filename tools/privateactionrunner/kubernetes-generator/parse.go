// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

func generateBundle(sdkRoot, target, source string, cfg targetConfig) (map[string][]byte, error) {
	clientDir := filepath.Join(sdkRoot, "k8s.io/client-go/kubernetes/typed", target, cfg.Version)
	clients, err := parser.ParseDir(token.NewFileSet(), clientDir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}
	apiDir := filepath.Join(sdkRoot, "k8s.io/api", target, cfg.Version)
	api, err := parser.ParseFile(token.NewFileSet(), filepath.Join(apiDir, "types.go"), nil, 0)
	if err != nil {
		return nil, err
	}
	interfaces := map[string]*ast.InterfaceType{}
	for _, pkg := range clients {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				if decl, ok := node.(*ast.TypeSpec); ok {
					if iface, ok := decl.Type.(*ast.InterfaceType); ok {
						interfaces[decl.Name.Name] = iface
					}
					return false
				}
				return true
			})
		}
	}
	var actions []action
	for _, name := range cfg.Resources {
		iface, ok := interfaces[name+"Interface"]
		if !ok {
			return nil, fmt.Errorf("no client interface for resource %s", name)
		}
		var plural string
		var namespaced bool
		for ifaceName, getter := range interfaces {
			if !strings.HasSuffix(ifaceName, "Getter") {
				continue
			}
			for _, method := range getter.Methods.List {
				fn, ok := method.Type.(*ast.FuncType)
				if !ok || len(method.Names) != 1 || fn.Results == nil || len(fn.Results.List) != 1 {
					continue
				}
				result, ok := fn.Results.List[0].Type.(*ast.Ident)
				if ok && result.Name == name+"Interface" {
					plural = method.Names[0].Name
					namespaced = len(fn.Params.List) > 0
				}
			}
		}
		if plural == "" {
			return nil, fmt.Errorf("no getter for resource %s", name)
		}
		fields, err := resourceFields(api, name, "k8s.io/api/"+target+"/"+cfg.Version)
		if err != nil {
			return nil, err
		}
		res := resource{Name: name, NamePluralized: plural, Namespaced: namespaced, Fields: fields}
		// Keep SDK order for stable registrations across regeneration.
		for _, method := range iface.Methods.List {
			if _, ok := method.Type.(*ast.FuncType); !ok || len(method.Names) != 1 {
				continue
			}
			if a, ok := methodAction(method.Names[0].Name, target, cfg, res); ok {
				actions = append(actions, a)
			}
		}
	}
	manual, err := manualActions(source)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	for _, a := range actions {
		if slices.Contains(manual, a.Name) {
			return nil, fmt.Errorf("action %s is both handwritten and generated", a.Name)
		}
		filename := snake(a.Name) + ".gen.go"
		content, err := render("actions", a)
		if err != nil {
			return nil, fmt.Errorf("render %s: %w", a.Name, err)
		}
		if _, exists := files[filename]; exists {
			return nil, fmt.Errorf("duplicate generated file %s", filename)
		}
		files[filename] = content
	}
	files["entrypoint.gen.go"], err = render("entrypoint", entrypoint{target, cfg.CopyrightYear, actions, manual})
	return files, err
}

func methodAction(method, target string, cfg targetConfig, res resource) (action, bool) {
	apiImport := "v1 " + strconv.Quote("k8s.io/api/"+target+"/"+cfg.Version)
	imports := map[string]bool{}
	var args []string
	name := strings.ToLower(method) + res.Name
	switch method {
	case "Create", "Update":
		args = []string{"ctx", "inputs.Body", "support.Meta" + method + "(inputs." + method + "Fields)"}
		imports[apiImport] = true
	case "Delete", "Get":
		args = []string{"ctx", "inputs.Name", "support.Meta" + method + "(inputs." + method + "Fields)"}
	case "DeleteCollection":
		args = []string{"ctx", "support.MetaDelete(inputs.DeleteFields)", "support.MetaList(inputs.ListFields)"}
		name = "deleteMultiple" + res.NamePluralized
		// Preserve the action catalog's acronym plural, e.g. ServiceCIDRS.
		if len(name) > 1 && name[len(name)-1] == 's' && name[len(name)-2] >= 'A' && name[len(name)-2] <= 'Z' {
			name = name[:len(name)-1] + "S"
		}
	case "List":
		args = []string{"ctx", "support.MetaList(inputs.ListFields)"}
		imports[apiImport] = true
		imports[`metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`] = true
	case "Patch":
		args = []string{"ctx", "inputs.Name", "typesv1.JSONPatchType", "body", "support.MetaPatch(inputs.PatchFields)"}
		imports[`"encoding/json"`] = true
		imports[`typesv1 "k8s.io/apimachinery/pkg/types"`] = true
	default:
		return action{}, false
	}
	if !slices.Contains([]string{"Delete", "DeleteCollection", "List"}, method) {
		for _, f := range res.Fields {
			for _, imp := range f.Imports {
				imports[imp] = true
			}
		}
	}
	namespaceRequired := res.Namespaced && method != "List"
	if namespaceRequired || slices.Contains([]string{"Create", "Update", "Get", "Delete", "Patch"}, method) {
		imports[`"errors"`] = true
	}
	var required, standard []string
	for imp := range imports {
		if imp == `"errors"` || imp == `"encoding/json"` {
			standard = append(standard, imp)
		} else {
			required = append(required, imp)
		}
	}
	slices.Sort(required)
	slices.Sort(standard)
	return action{
		Name: name, Package: target, Client: title(target) + title(cfg.Version),
		CopyrightYear: cfg.CopyrightYear, Resource: res, SDKMethod: method,
		SDKMethodArgs: args, RequiredImports: required, StandardImports: standard,
		NamespaceRequired: namespaceRequired,
	}, true
}

func resourceFields(api *ast.File, name, apiPath string) ([]field, error) {
	imports := map[string]string{}
	for _, imp := range api.Imports {
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		alias := filepath.Base(p)
		if imp.Name != nil {
			alias = imp.Name.Name
		}
		imports[alias] = p
	}
	for _, decl := range api.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			if ts.Name.Name != name {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return nil, fmt.Errorf("resource %s is not a struct", name)
			}
			var result []field
			for _, f := range st.Fields.List {
				fieldName := ""
				if len(f.Names) > 0 {
					fieldName = f.Names[0].Name
				} else if selector, ok := f.Type.(*ast.SelectorExpr); ok {
					fieldName = selector.Sel.Name
				}
				if fieldName == "TypeMeta" {
					continue
				}
				if fieldName == "" {
					return nil, fmt.Errorf("unsupported embedded field in %s", name)
				}
				fullType, required, err := qualifyType(f.Type, apiPath, imports)
				if err != nil {
					return nil, fmt.Errorf("field %s.%s: %w", name, fieldName, err)
				}
				tag := ""
				if f.Tag != nil {
					tag = f.Tag.Value
				}
				result = append(result, field{fieldName, fullType, tag, required})
			}
			return result, nil
		}
	}
	return nil, fmt.Errorf("resource type %s not found", name)
}

// qualifyType qualifies local API types while preserving external import aliases.
func qualifyType(expr ast.Expr, apiPath string, imports map[string]string) (string, []string, error) {
	var required []string
	var qualify func(ast.Expr) (string, error)
	qualify = func(expr ast.Expr) (string, error) {
		switch t := expr.(type) {
		case *ast.Ident:
			if ast.IsExported(t.Name) {
				required = append(required, "v1 "+strconv.Quote(apiPath))
				return "v1." + t.Name, nil
			}
			return t.Name, nil
		case *ast.SelectorExpr:
			pkg, ok := t.X.(*ast.Ident)
			if !ok || imports[pkg.Name] == "" {
				return "", fmt.Errorf("unresolved selector %v", t)
			}
			imp := strconv.Quote(imports[pkg.Name])
			if pkg.Name != filepath.Base(imports[pkg.Name]) {
				imp = pkg.Name + " " + imp
			}
			required = append(required, imp)
			return pkg.Name + "." + t.Sel.Name, nil
		case *ast.StarExpr:
			inner, err := qualify(t.X)
			return "*" + inner, err
		case *ast.ArrayType:
			var length bytes.Buffer
			if t.Len != nil {
				if err := format.Node(&length, token.NewFileSet(), t.Len); err != nil {
					return "", err
				}
			}
			inner, err := qualify(t.Elt)
			return "[" + length.String() + "]" + inner, err
		case *ast.MapType:
			key, err := qualify(t.Key)
			if err != nil {
				return "", err
			}
			value, err := qualify(t.Value)
			return "map[" + key + "]" + value, err
		default:
			return "", fmt.Errorf("unsupported API field type %T", expr)
		}
	}
	fullType, err := qualify(expr)
	return fullType, required, err
}

func manualActions(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".gen.go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			return nil, err
		}
		// Only constructors define actions; support files must not become registrations.
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "New") || !strings.HasSuffix(fn.Name.Name, "Handler") {
				continue
			}
			actionName := strings.TrimSuffix(strings.TrimPrefix(fn.Name.Name, "New"), "Handler")
			if actionName == "" {
				continue
			}
			actionName = strings.ToLower(actionName[:1]) + actionName[1:]
			if slices.Contains(names, actionName) {
				return nil, fmt.Errorf("duplicate manual action %s", actionName)
			}
			names = append(names, actionName)
		}
	}
	slices.Sort(names)
	return names, nil
}
