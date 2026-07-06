// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main generates docs/CONFIG.md, a reference of every Pulumi config
// key read by the e2e framework (e.g. "ddinfra:gcp/defaultPrivateKeyPath").
//
// Config keys are declared as string constants scattered across
// common/config/environment.go and resources/*/environment.go, each read
// through a *sdkconfig.Config field whose namespace ("ddinfra", "aws", ...)
// is set once via sdkconfig.New(ctx, <namespaceConst>). This tool parses the
// source (not compiled code) to recover, for every key constant, which
// field read it and which namespace that field belongs to.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const outputPath = "docs/CONFIG.md"

// configKey is one resolved Pulumi config key.
type configKey struct {
	Namespace string // e.g. "ddinfra"
	Value     string // e.g. "gcp/defaultPrivateKeyPath"
	Const     string // e.g. "DDInfraDefaultPrivateKeyPath"
	Doc       string
	File      string
	Line      int
}

// configCallSite pairs a config key constant with the struct field it was
// read through, e.g. {Field: "InfraConfig", ConstIdent: "DDInfraOSDescriptor"}.
type configCallSite struct {
	Field      string
	ConstIdent string
}

func main() {
	check := flag.Bool("check", false, "exit 1 if docs/CONFIG.md is out of date instead of writing it")
	flag.Parse()

	root, err := frameworkRoot()
	if err != nil {
		fatal(err)
	}

	files, err := discoverFiles(root)
	if err != nil {
		fatal(err)
	}

	// Each provider redeclares identifiers like DDInfraDefaultPrivateKeyPath
	// with a different value (aws/, gcp/, az/, ...), so const lookups and
	// call-site matching must stay scoped to a single file's *ast.File —
	// merging them into one global map-by-identifier would let later files
	// silently clobber earlier ones. Only namespaceByField (keyed by struct
	// field name, e.g. "InfraConfig") and the final resolved keys are safe
	// to accumulate across files, since those never collide.
	fset := token.NewFileSet()
	namespaceByField := make(map[string]string)
	var keys []configKey
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			fatal(fmt.Errorf("parsing %s: %w", path, err))
		}

		constValues, constPos := collectConstDecls(fset, file)
		maps.Copy(namespaceByField, resolveNamespacesByField(constValues, file))

		callSites := findConfigCallSites(file)
		keys = append(keys, buildConfigKeys(callSites, constPos, namespaceByField, root)...)
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Namespace != keys[j].Namespace {
			return keys[i].Namespace < keys[j].Namespace
		}
		return keys[i].Value < keys[j].Value
	})

	doc := renderMarkdown(keys)

	if *check {
		existing, err := os.ReadFile(filepath.Join(root, outputPath))
		if err != nil {
			fatal(fmt.Errorf("reading %s: %w (run `go run ./cmd/configdoc` to generate it)", outputPath, err))
		}
		if string(existing) != doc {
			fmt.Fprintf(os.Stderr, "%s is out of date; run `go run ./cmd/configdoc` and commit the result\n", outputPath)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(filepath.Join(root, outputPath), []byte(doc), 0o644); err != nil {
		fatal(err)
	}
}

// frameworkRoot returns the directory containing this module's go.mod
// (test/e2e-framework), regardless of the working directory this binary is
// invoked from.
func frameworkRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// discoverFiles returns every environment.go that declares Pulumi config
// keys: the shared common/config one, plus one per cloud/local provider
// under resources/.
func discoverFiles(root string) ([]string, error) {
	var files []string
	if _, err := os.Stat(filepath.Join(root, "common/config/environment.go")); err == nil {
		files = append(files, filepath.Join(root, "common/config/environment.go"))
	}
	matches, err := filepath.Glob(filepath.Join(root, "resources/*/environment.go"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	files = append(files, matches...)
	return files, nil
}

// collectConstDecls indexes every string-valued const declaration in file by
// identifier, along with its source position and any attached comment (used
// later as the key's documentation). Scoped to a single file: identifiers
// like DDInfraDefaultPrivateKeyPath are redeclared with different values in
// every provider's environment.go, so merging across files would collide.
func collectConstDecls(fset *token.FileSet, file *ast.File) (values map[string]string, positions map[string]configKey) {
	values = make(map[string]string)
	positions = make(map[string]configKey)

	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			name := vs.Names[0].Name
			value := strings.Trim(lit.Value, `"`)
			values[name] = value

			var docParts []string
			if vs.Doc != nil {
				docParts = append(docParts, strings.TrimSpace(vs.Doc.Text()))
			}
			if vs.Comment != nil {
				docParts = append(docParts, strings.TrimSpace(vs.Comment.Text()))
			}

			pos := fset.Position(vs.Pos())
			positions[name] = configKey{
				Const: name,
				Value: value,
				Doc:   strings.TrimSpace(strings.Join(docParts, " ")),
				File:  pos.Filename,
				Line:  pos.Line,
			}
		}
	}
	return values, positions
}

// resolveNamespacesByField finds every `field: sdkconfig.New(ctx, <ns>)`
// assignment in file (in CommonEnvironment and any provider-specific
// environment, e.g. resources/aws's separate "aws" namespace) and returns
// the field name -> namespace value mapping, e.g. {"InfraConfig": "ddinfra"}.
func resolveNamespacesByField(constValues map[string]string, file *ast.File) map[string]string {
	result := make(map[string]string)
	ast.Inspect(file, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		field, ok := kv.Key.(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := kv.Value.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "New" || len(call.Args) != 2 {
			return true
		}
		ns := resolveStringArg(call.Args[1], constValues)
		if ns != "" {
			result[field.Name] = ns
		}
		return true
	})
	return result
}

// resolveStringArg returns the literal string value of expr, whether it's a
// raw string literal or an identifier referencing a known const.
func resolveStringArg(expr ast.Expr, constValues map[string]string) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			return strings.Trim(e.Value, `"`)
		}
	case *ast.Ident:
		return constValues[e.Name]
	}
	return ""
}

// findConfigCallSites scans every function body in file for expressions that
// read a Pulumi config value, and returns one configCallSite per (field,
// const) pair it recognizes. It must handle two call shapes seen in this
// codebase:
//
//   - Direct:  e.InfraConfig.Get(DDInfraOSDescriptor)
//     e.AgentConfig.RequireSecret(DDAgentAPIKeyParamName)
//     (method one of Get, Require, RequireSecret, Try, TryBool, TryInt, TryObject;
//     receiver is a SelectorExpr like `e.InfraConfig` — the field is its .Sel.Name;
//     the sole argument is an *ast.Ident naming the const)
//
//   - Wrapper: e.GetBoolWithDefault(e.InfraConfig, DDInfraOSImageIDUseLatest, false)
//     (method name ends in "WithDefault"; first argument is the field
//     selector, second argument is the const identifier)
//
// Anything that doesn't match either shape (e.g. .Getenv, calls on a
// variable that isn't a `<recv>.<Field>` selector) should be skipped, not
// guessed at.
func findConfigCallSites(file *ast.File) []configCallSite {
	directMethods := map[string]bool{
		"Get": true, "Require": true, "RequireSecret": true,
		"Try": true, "TryBool": true, "TryInt": true, "TryObject": true,
	}

	var sites []configCallSite
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if directMethods[sel.Sel.Name] && len(call.Args) >= 1 {
				if field := fieldName(sel.X); field != "" {
					if constIdent, ok := call.Args[0].(*ast.Ident); ok {
						sites = append(sites, configCallSite{Field: field, ConstIdent: constIdent.Name})
					}
				}
				return true
			}
			if strings.HasSuffix(sel.Sel.Name, "WithDefault") && len(call.Args) >= 2 {
				if field := fieldName(call.Args[0]); field != "" {
					if constIdent, ok := call.Args[1].(*ast.Ident); ok {
						sites = append(sites, configCallSite{Field: field, ConstIdent: constIdent.Name})
					}
				}
			}
		}
		return true
	})
	return sites
}

// fieldName returns the field name of a `<recv>.<Field>` selector (e.g.
// "InfraConfig" for `e.InfraConfig`), or "" if expr isn't that shape.
func fieldName(expr ast.Expr) string {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return sel.Sel.Name
}

// buildConfigKeys joins call sites, const declarations, and namespace
// resolution into the final sorted list of documented keys.
func buildConfigKeys(callSites []configCallSite, positions map[string]configKey, namespaceByField map[string]string, root string) []configKey {
	seen := make(map[string]bool)
	var keys []configKey
	for _, cs := range callSites {
		if seen[cs.ConstIdent] {
			continue
		}
		ns, ok := namespaceByField[cs.Field]
		if !ok {
			fmt.Fprintf(os.Stderr, "configdoc: warning: no namespace resolved for field %q (const %s); skipping\n", cs.Field, cs.ConstIdent)
			continue
		}
		base, ok := positions[cs.ConstIdent]
		if !ok {
			fmt.Fprintf(os.Stderr, "configdoc: warning: const %s referenced but not found as a string decl; skipping\n", cs.ConstIdent)
			continue
		}
		seen[cs.ConstIdent] = true
		base.Namespace = ns
		if rel, err := filepath.Rel(root, base.File); err == nil {
			base.File = rel
		}
		keys = append(keys, base)
	}
	return keys
}

func renderMarkdown(keys []configKey) string {
	var b bytes.Buffer
	b.WriteString("<!-- Generated by `go run ./cmd/configdoc`. Do not edit by hand. -->\n\n")
	b.WriteString("# Pulumi config key reference\n\n")
	b.WriteString("Every Pulumi config key read by the e2e framework, grouped by namespace. ")
	b.WriteString("Set one with `pulumi config set <namespace>:<key> <value>` or `-c <namespace>:<key>=<value>`.\n")

	var currentNamespace string
	for _, k := range keys {
		if k.Namespace != currentNamespace {
			currentNamespace = k.Namespace
			fmt.Fprintf(&b, "\n## %s\n\n", currentNamespace)
			b.WriteString("| Key | Const | Defined in | Doc |\n")
			b.WriteString("|---|---|---|---|\n")
		}
		doc := strings.ReplaceAll(k.Doc, "\n", " ")
		fmt.Fprintf(&b, "| `%s:%s` | `%s` | %s:%d | %s |\n", currentNamespace, k.Value, k.Const, k.File, k.Line, doc)
	}
	return b.String()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "configdoc:", err)
	os.Exit(1)
}
